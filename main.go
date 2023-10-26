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

type webHandler struct {
	sync.RWMutex
	activeLocks map[string]time.Time
}

func acquireLock(name string, timeout time.Duration) error {

	log.Printf("Acquiring lock %s", name)

	u := url.Values{
		"name":    []string{name},
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

func releaseLock(name string) error {

	log.Printf("Releasing lock %s", name)
	resp, err := http.Get("http://localhost:80/release-lock?name=" + name)
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

	timeout := req.Form.Get("timeout")

	fmt.Println("---------" + name + "----------" + timeout)
	if timeout == "" {
		http.Error(w, "timeout in seconds is required", http.StatusBadRequest)
		return
	}

	fmt.Println("............", timeout)
	timeoutSec, err := strconv.ParseInt(timeout, 10, 64)
	fmt.Println("timeout in seconds ", timeoutSec)
	if err != nil || timeoutSec < 0 {
		http.Error(w, "timeout must be integer", http.StatusBadRequest)
		return
	}

	h.Lock()
	defer h.Unlock()

	now := time.Now()

	if endTime, ok := h.activeLocks[name]; ok && endTime.After(now) {
		http.Error(w, fmt.Sprintf("lock is already active (%s left)", endTime.Sub(now)), http.StatusConflict)
		return
	}

	fmt.Println("name of lock acquired is " + name)

	h.activeLocks[name] = time.Now().Add(time.Duration(timeoutSec) * time.Second)
	w.Write([]byte("Success"))
}

func (h *webHandler) ReleaseLock(w http.ResponseWriter, req *http.Request) {

	fmt.Println("We are inside release lock........")
	req.ParseForm()

	name := req.Form.Get("name")

	fmt.Println("inside ........................ ", name)
	if name == "" {
		http.Error(w, "lock name is required", http.StatusBadRequest)
		return
	}

	h.Lock()
	defer h.Unlock()
	fmt.Println("name of lock is ", name)
	delete(h.activeLocks, name)
	fmt.Println(h.activeLocks)
	fmt.Println("lock has been released.....")
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
		w.Write([]byte(name + "\n"))
	}

}

func main() {

	flag.Parse()

	if *server {
		h := &webHandler{
			activeLocks: make(map[string]time.Time),
		}
		fmt.Println("Server is up and running .......")
		http.Handle("/", http.HandlerFunc(h.listLocks))
		http.Handle("/acquire-lock", http.HandlerFunc(h.AcquireLock))
		http.Handle("/release-lock", http.HandlerFunc(h.ReleaseLock))
		log.Fatal(http.ListenAndServe(":80", nil))
	}

	if err := acquireLock(*name, time.Minute); err != nil {
		log.Fatal(err)
	}

	defer releaseLock(*name)
	ch := make(chan os.Signal, 5)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-ch
		releaseLock(*name)
		os.Exit(0)
	}()

	log.Printf("Sleeping for one minute")
	time.Sleep(time.Second * 30)

}
