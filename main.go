package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"
)

var server = flag.Bool("server", false, "Run as server")
var name = flag.String("name", "default_lock", "Name of the lock")
var errLocked = errors.New("Lock is already active")

type webHandler struct {
	sync.RWMutex
	activeLocks map[string]bool
}

func acquireLock(name string) error {

	resp, err := http.Get("http://localhost:80/acquire-lock?name=" + (url.Values{"name": []string{name}}).Encode())
	fmt.Println("resp is ", resp)
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
	resp, err := http.Get("http://localhost:80/acquire-lock?name=" + name)
	if err != nil {
		return fmt.Errorf("making http query %v", err)
	}

	defer resp.Body.Close()
	respBody, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusOK {
		return nil
	}
	return fmt.Errorf("unexpected response: status code %d, contents: %v", resp.StatusCode, string(respBody))
}

func (h *webHandler) AcquireLock(w http.ResponseWriter, req *http.Request) {
	req.ParseForm()

	name := req.Form.Get("name")

	if name == "" {
		http.Error(w, "lock name is required", http.StatusBadRequest)
		return
	}

	h.Lock()
	defer h.Unlock()

	if h.activeLocks[name] != false {
		http.Error(w, "lock is already active", http.StatusConflict)
		return
	}
	h.activeLocks[name] = true
	w.Write([]byte("Success"))
}

func (h *webHandler) ReleaseLock(w http.ResponseWriter, req *http.Request) {

	req.ParseForm()

	name := req.Form.Get("name")

	if name == "" {
		http.Error(w, "lock name is required", http.StatusBadRequest)
		return
	}

	h.Lock()
	defer h.Unlock()

	delete(h.activeLocks, name)
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
			activeLocks: make(map[string]bool),
		}
		fmt.Println("Server is up and running .......")
		http.Handle("/", http.HandlerFunc(h.listLocks))
		http.Handle("/acquire-lock", http.HandlerFunc(h.AcquireLock))
		http.Handle("/release-lock", http.HandlerFunc(h.ReleaseLock))
		log.Fatal(http.ListenAndServe(":80", nil))
	}

	if err := acquireLock(*name); err != nil {
		log.Fatal(err)
	}
	defer releaseLock(*name)
	log.Printf("Sleeping for one minute")
	time.Sleep(time.Second)

}

func startServer() {

}
