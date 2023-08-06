package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/rhaidiz/yves"
)

var redactedForm = "xxxxx"
var forbiddenWord = "password"

func main() {

	proxy := yves.NewProxy()

	// intercept websocket fragment and if they contain forbiddenWord, replace it
	// with a redacted version
	proxy.HandleWebSocRequest = func(webFrag *yves.WebsocketFragment) *yves.WebsocketFragment {
		// this is a final fragment of type text
		if webFrag.FinBit && webFrag.OpCode == 1 {
			message := string(webFrag.Data)
			// print received fragment
			fmt.Printf("message: %s", webFrag.Data)
			if strings.Contains(message, forbiddenWord) {
				newMessage := strings.ReplaceAll(message, forbiddenWord, redactedForm)
				// update message length
				webFrag.PayloadLength = uint64(len(newMessage))
				// replace data
				webFrag.Data = []byte(newMessage)
			}
		}
		return webFrag
	}

	log.Fatal(http.ListenAndServe("127.0.0.1:8080", proxy))
}
