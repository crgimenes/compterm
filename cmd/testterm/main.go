package main

import (
	"bufio"
	"io"
	"log"
	"os"
)

func main() {

	// reset terminal
	os.Stdout.Write([]byte("\033c"))

	// set terminal size to 40x20
	os.Stdout.Write([]byte("\033[8;40;20t"))

	f, err := os.Open("out2.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	// read the file byte by byte
	r := bufio.NewReader(f)
	for {
		b, err := r.ReadByte()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}
		os.Stdout.Write([]byte{b})
	}
}
