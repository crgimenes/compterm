package playback

import (
	"compterm/config"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"time"
)

type Data struct {
	CMD     byte
	Payload []byte
	Time    int64
}

// rec data im csv format
func Rec(cmd byte, payload []byte) {
	file := config.CFG.PlaybackFile

	// Open file for appending. The file is created if it doesn't exist.
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("error opening file: %s\r\n", err)
	}

	// Make sure to close the file when you're done
	defer f.Close()

	// Cria um escritor CSV
	writer := csv.NewWriter(f)
	writer.Comma = ';'

	defer writer.Flush()

	data := []string{fmt.Sprintf("%d", cmd), string(payload), fmt.Sprintf("%d", time.Now().UnixNano())}
	err = writer.Write(data)
	if err != nil {
		log.Fatalln("error writing record to file", err)
	}
}

func Play(w io.Writer) error {
	file := config.CFG.PlaybackFile

	// Open file for appending. The file is created if it doesn't exist.
	f, err := os.OpenFile(file, os.O_RDONLY, 0644)
	if err != nil {
		return fmt.Errorf("error opening file: %s\r\n", err)
	}

	// Make sure to close the file when you're done
	defer f.Close()

	// Cria um leitor CSV
	reader := csv.NewReader(f)
	reader.Comma = ';'

	data := Data{}
	lastTime := int64(0)

	for {
		// TODO: add a way to stop the playback

		// Read one record (until the next delimiter) from csvReader.
		record, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading record to file: %s\r\n", err)
		}

		c, err := strconv.ParseInt(record[0], 10, 8)
		if err != nil {
			return fmt.Errorf("error parsing record to file: %s\r\n", err)
		}

		data.CMD = byte(c)

		data.Payload = []byte(record[1])
		t, err := strconv.ParseInt(record[2], 10, 64)
		if err != nil {
			return fmt.Errorf("error parsing record to file: %s\r\n", err)
		}

		data.Time = t

		if lastTime == 0 {
			lastTime = data.Time
		}

		// wait to send
		time.Sleep(time.Duration(data.Time-lastTime) * time.Nanosecond)

		lastTime = data.Time

		// send data
		n := len(data.Payload) + 1
		buffer := make([]byte, n)
		buffer[0] = data.CMD
		copy(buffer[1:], data.Payload)

		_, err = w.Write(buffer[:n])
		if err != nil {
			return fmt.Errorf("error writing to output: %s\r\n", err)
		}
	}

	return nil
}
