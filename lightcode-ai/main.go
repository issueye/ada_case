// Command lightcode-ai is the editor's assistant as its own project: a
// process the editor spawns on the first question, which reads the
// protocol on its standard input and streams an OpenAI-compatible answer
// on its standard output.
//
// It has its own go.mod and builds on its own, so the editor's window
// never links the network client, and a plugin bug cannot take the editor
// with it.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"lightcode.ai/client/protocol"
	"lightcode.ai/client/serve"
)

func main() {
	version := flag.Bool("version", false, "print the plugin's name and version, then exit")
	flag.Parse()
	if *version {
		fmt.Println(protocol.Name, protocol.Version)
		return
	}
	// Standard output is the protocol: every diagnostic goes to standard
	// error, or a log line would be read as a message.
	log.SetFlags(0)
	log.SetOutput(os.Stderr)
	if err := serve.Run(serve.Options{Stderr: os.Stderr}); err != nil {
		log.Fatal(err)
	}
}
