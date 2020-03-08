package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/kgeonw/hyperledger-fabric-prototype/hyperledger"
)

type Message struct {
	key   string
	value string
}

// Start & Test
func main() {
	hyperledger.StartFabric()

	var inputKey, inputVal string
	fmt.Print("Enter (key value): ")
	_, err := fmt.Scan(&inputKey, &inputVal)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Println(inputKey, inputVal)

	// inputKey = "1"
	// inputVal = "first"

	hyperledger.WriteTrans(inputKey, inputVal)
	// hyperledger.WriteTrans("2", "second")
	// hyperledger.WriteTrans("3", "third")

	time.Sleep(1 * time.Second)

	result1 := hyperledger.GetTrans(inputKey)
	// result2 := hyperledger.GetTrans("2")
	// result3 := hyperledger.GetTrans("3")

	time.Sleep(1 * time.Second)

	fmt.Printf("Result - key: %s value: %s\n", inputKey, result1)
	// fmt.Printf("key2 : %s \n", result2)
	// fmt.Printf("key3 : %s \n", result3)

	//web_server_run()
}

func web_server_run() error {
	mux := makeMuxRouter()
	httpPort := "8080"
	log.Println("HTTP Server Listening on port :", httpPort)
	s := &http.Server{
		Addr:           ":" + httpPort,
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	if err := s.ListenAndServe(); err != nil {
		return err
	}

	return nil
}

func makeMuxRouter() http.Handler {
	muxRouter := mux.NewRouter()
	muxRouter.HandleFunc("/", getLetter).Methods("GET")
	muxRouter.HandleFunc("/", writeLetter).Methods("POST")
	return muxRouter
}

func getLetter(w http.ResponseWriter, r *http.Request) {

	letters := hyperledger.GetTrans("1")
	bytes, err := json.MarshalIndent(letters, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	io.WriteString(w, string(bytes))
}

func writeLetter(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var msg Message

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&msg); err != nil {
		respondWithJSON(w, r, http.StatusBadRequest, r.Body)
		return
	}
	defer r.Body.Close()

	result := hyperledger.WriteTrans(msg.key, msg.value)
	respondWithJSON(w, r, http.StatusCreated, result)

}

func respondWithJSON(w http.ResponseWriter, r *http.Request, code int, payload interface{}) {
	response, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("HTTP 500: Internal Server Error"))
		return
	}
	w.WriteHeader(code)
	w.Write(response)
}
