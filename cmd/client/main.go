package main

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os/exec"
	"strconv"
)

func main() {
	flag.Parse()

	dial := flag.Arg(0)
	videoStream := flag.Arg(1)

	socketName := rand.Text()
	socketPath := fmt.Sprintf("/tmp/%s", socketName)

	log.Printf("creating socket at %s", socketPath)

	cmd := exec.Command("mpv", videoStream, fmt.Sprintf("--input-ipc-server=%s", socketPath), "--force-window=immediate", "--pause")
	log.Print(cmd)

	done := make(chan error)
	go func() {
		done <- cmd.Run()
	}()

	var mpv net.Conn
	err := errors.New("unix socket connection no initiated")
	for err != nil {
		select {
		case err := <-done:
			if err != nil {
				log.Fatal(err)
			}

		default:
			mpv, err = net.Dial("unix", socketPath)
		}
	}

	defer mpv.Close()

	conn, err := net.Dial("tcp", dial)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	couple := &ConnCouple{
		net: conn,
		mpv: mpv,
	}

	go handleNet(couple)
	go handleMpv(couple)

	if err := <-done; err != nil {
		log.Fatal(err)
	}
}

type ConnCouple struct {
	net net.Conn
	mpv net.Conn

	hasSeeked bool
	hasPaused bool
}

func handleMpv(couple *ConnCouple) {
	const observePause = "{ \"command\": [\"observe_property\", 1, \"pause\"] }\n"
	const getTime = "{ \"command\": [\"get_property\", \"time-pos\"] }\n"
	couple.mpv.Write([]byte(observePause))
	mpvReader := bufio.NewReader(couple.mpv)

	for {
		unixMessage, err := mpvReader.ReadBytes('\n')
		if err != nil {
			log.Print("failed to read from connection:", err)
			break
		}

		blob := make(map[string]any)
		if err := json.Unmarshal(unixMessage, &blob); err != nil {
			log.Fatal(err)
		}

		event, eventExists := blob["event"]
		property, propertyChangeExists := blob["name"]
		data, dataExists := blob["data"]
		// id, idExists := blob["request_id"]

		log.Print(string(unixMessage))

		if eventExists && event == "property-change" && propertyChangeExists && property == "pause" && dataExists {
			isPaused, ok := (data).(bool)
			if !ok {
				log.Fatalf("failed to coerce pause boolean indicator to boolean type: %v", data)
			}

			if couple.hasPaused {
				couple.hasPaused = false
				continue
			}

			bytesToWrite := []byte(fmt.Sprintf("%t\n", isPaused))
			log.Print("mpv: writing pause to net")
			_, err = couple.net.Write(bytesToWrite)
			if err != nil {
				log.Print(err)
			}

		} else if eventExists && event == "seek" {
			couple.mpv.Write([]byte(getTime))
		} else if error, exists := blob["error"]; exists {
			if error != "success" {
				log.Print("seek time request failed: %s", error)
			}

			if dataExists && data != nil {

				playHeadTime, ok := data.(float64)
				if !ok {
					log.Fatalf("failed to coerce seek playhead timestamp to float64 type: %v", data)
				}

				if couple.hasSeeked {
					couple.hasSeeked = false
					continue
				}

				bytesToWrite := []byte(fmt.Sprintf("%f\n", playHeadTime))
				log.Print("mpv: writing seek to net")
				_, err = couple.net.Write(bytesToWrite)
				couple.net.Write(bytesToWrite)
				if err != nil {
					log.Print(err)
				}
			}
		}

	}
}

func handleNet(couple *ConnCouple) {

	reader := bufio.NewReader(couple.net)

	for {
		message, err := reader.ReadBytes('\n')
		if err != nil {
			log.Print("failed to read from connection:", err)
			break
		}

		messageStr := string(message[:len(message)-1])

		log.Printf("net: %s", message)
		time, err := strconv.ParseFloat(messageStr, 64)
		if err == nil {
			command := fmt.Sprintf("{ \"command\": [\"seek\", %f, \"absolute\"] }\n", time)
			couple.mpv.Write([]byte(command))
			couple.hasSeeked = true
			// couple.id += 1
			continue
		}

		pause, err := strconv.ParseBool(messageStr)
		if err == nil {
			command := fmt.Sprintf("{ \"command\": [\"set_property\", \"pause\", %t] }\n", pause)
			couple.mpv.Write([]byte(command))
			couple.hasPaused = true
			// couple.id += 1
			continue
		}

		log.Print("net parsing error: ", err)
	}
}
