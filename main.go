package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"
)

var server = flag.Bool("server", false, "Run as server")
var name = flag.String("name", "default_lock", "Name of the lock")
var errLocked = errors.New("Lock is already active")

type lockInfo struct {
	owner string
	end   time.Time
}

type webHandler struct {
	sync.RWMutex
	activeLocks map[string]lockInfo
}

func acquireLock(name string, owner string, timeout time.Duration) error {

	log.Printf("Acquiring lock %s", name)

	u := url.Values{
		"name":    []string{name},
		"owner":   []string{owner},
		"timeout": []string{strconv.Itoa(int(timeout / time.Second))},
	}

	fmt.Println("u.Encode is ", u.Encode())

	resp, err := http.Get("http://localhost:80/acquire-lock?" + u.Encode())
	if err != nil {
		return fmt.Errorf("making http query %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("resp is ", respBody)
	if resp.StatusCode == http.StatusConflict {
		return errLocked
	} else if resp.StatusCode == http.StatusOK {
		return nil
	}
	return fmt.Errorf("unexpected response: status code %d, contents: %v", resp.StatusCode, string(respBody))
}

func releaseLock(name string, owner string) error {

	u := url.Values{
		"name":  []string{name},
		"owner": []string{owner},
	}
	log.Printf("Releasing lock %s", name)

	resp, err := http.Get("http://localhost:80/release-lock?" + u.Encode())
	if err != nil {
		return fmt.Errorf("making http query %v", err)
	}

	defer resp.Body.Close()
	respBody, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("release lock called")

	if resp.StatusCode == http.StatusOK {
		return nil
	}
	fmt.Println("status code is ", resp.StatusCode)
	return fmt.Errorf("unexpected response: status code %d, contents: %v", resp.StatusCode, string(respBody))
}

func (h *webHandler) AcquireLock(w http.ResponseWriter, req *http.Request) {
	req.ParseForm()

	name := req.Form.Get("name")

	if name == "" {
		http.Error(w, "lock name is required", http.StatusBadRequest)
		return
	}

	owner := req.Form.Get("owner")

	if owner == "" {
		http.Error(w, "Lock Owner name is required", http.StatusBadRequest)
		return
	}

	timeout := req.Form.Get("timeout")

	if timeout == "" {
		http.Error(w, "timeout in seconds is required", http.StatusBadRequest)
		return
	}

	timeoutSec, err := strconv.ParseInt(timeout, 10, 64)

	if err != nil || timeoutSec < 0 {
		http.Error(w, "timeout must be integer", http.StatusBadRequest)
		return
	}

	h.Lock()
	defer h.Unlock()

	now := time.Now()

	li, ok := h.activeLocks[name]

	// if satya is the owner of lock3 and lock3 has not yet expired and john tries to access the lock then he cannot
	// but if satya tries to acquire the same lock again ,he can and now the expiration time goes up
	if ok && li.owner != owner && li.end.After(now) {
		http.Error(w, fmt.Sprintf("lock is already active (%s left)", li.end.Sub(now)), http.StatusConflict)
		return
	}

	h.activeLocks[name] = lockInfo{
		owner: owner,
		end:   time.Now().Add(time.Duration(timeoutSec) * time.Second),
	}
	w.Write([]byte("Success"))
}

func (h *webHandler) ReleaseLock(w http.ResponseWriter, req *http.Request) {

	req.ParseForm()

	name := req.Form.Get("name")

	if name == "" {
		http.Error(w, "lock name is required", http.StatusBadRequest)
		return
	}

	owner := req.Form.Get("owner")

	if owner == "" {
		http.Error(w, "Lock Owner name is required", http.StatusBadRequest)
		return
	}

	h.Lock()
	defer h.Unlock()

	if li, ok := h.activeLocks[name]; ok && li.owner != owner {
		http.Error(w, fmt.Sprintf("lock has another owner %q", li.owner), http.StatusBadRequest)
		return
	}

	fmt.Println("lock being deleted is " + name)
	delete(h.activeLocks, name)
	fmt.Println(h.activeLocks)

	w.Write([]byte("Success releasing lock"))

}

func (h *webHandler) listLocks(w http.ResponseWriter, req *http.Request) {

	h.Lock()
	defer h.Unlock()
	w.Write([]byte("Active Locks" + "\n"))

	locks := make([]string, 0, len(h.activeLocks))

	for name, _ := range h.activeLocks {
		locks = append(locks, name)
	}

	sort.Strings(locks)

	for _, name := range locks {
		fmt.Fprintf(w, "%s: owned by %q, expires in %s\n", name, h.activeLocks[name].owner, h.activeLocks[name].end.Sub(time.Now()))
	}

}

func main() {

	flag.Parse()

	if *server {
		h := &webHandler{
			activeLocks: make(map[string]lockInfo),
		}
		fmt.Println("Server is up and running .......")
		http.Handle("/", http.HandlerFunc(h.listLocks))
		http.Handle("/acquire-lock", http.HandlerFunc(h.AcquireLock))
		http.Handle("/release-lock", http.HandlerFunc(h.ReleaseLock))
		log.Fatal(http.ListenAndServe(":80", nil))
	}

	hostname, _ := os.Hostname()
	owner := fmt.Sprintf("%s:%d", hostname, os.Getpid())

	fmt.Println("owner is ", owner)

	if err := acquireLock(*name, owner, 1*time.Minute); err != nil {
		log.Fatal(err)
	}

	defer releaseLock(*name, owner)
	ch := make(chan os.Signal, 5)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-ch
		releaseLock(*name, owner)
		os.Exit(0)
	}()

	log.Printf("Sleeping for one minute")
	time.Sleep(time.Second * 60)

}
