package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
	"unsafe"
)

const (
	devLocation     = "/dev/"
	checkMMCDevName = "mmcblk0"
	sendCMD56Arg    = 1
)

type sandiskCMD56Response [512]byte

func (s *sandiskCMD56Response) sdIdentifier() string {
	return fmt.Sprintf("0x%X%X", s[0], s[1])
}

func (s *sandiskCMD56Response) manufactureDate() time.Time {
	dateBytes := s[2:8]
	dateStr := *(*string)(unsafe.Pointer(&dateBytes))
	time, _ := time.Parse("060102", dateStr)
	return time
}

func (s *sandiskCMD56Response) healthStatus() int {
	return int(s[8])
}

func (s *sandiskCMD56Response) featureRevision() string {
	return fmt.Sprintf("%08b %08b", int(s[11]), int(s[12]))
}

func (s *sandiskCMD56Response) generationIdentifier() string {
	return fmt.Sprintf("0x%02X", s[14])
}

func (s *sandiskCMD56Response) programmableProductString() string {
	return strings.TrimRight(string(s[49:81]), " ")
}

func (s sandiskCMD56Response) json() []byte {
	jsonBytes, _ := json.Marshal(map[string]interface{}{
		"SD Identifier":               s.sdIdentifier(),
		"Manufacture date":            s.manufactureDate().Format("2006-01-02"),
		"Health Status in % used":     s.healthStatus(),
		"Feature Revision":            s.featureRevision(),
		"Generation Identifier":       s.generationIdentifier(),
		"Programmable Product String": s.programmableProductString(),
	})
	return jsonBytes
}

func parseCMD56(input []byte) (res *sandiskCMD56Response, err error) {
	inputString := (*string)(unsafe.Pointer(&input))
	inputBytes := strings.Fields(*inputString)[1:]
	if len(inputBytes) != 512 {
		return nil, errors.New("this MMC not made by Sandisk")
	}
	res = &sandiskCMD56Response{}
	for idx, inputByte := range inputBytes {
		if(len(inputByte) == 1) {
			inputByte = "0" + inputByte
		}
		bs, err := hex.DecodeString(inputByte)
		if err != nil {
			return nil, err
		}
		res[idx] = bs[0]
	}
	return res, nil
}

func response() (result []byte, err error) {
	devPath := path.Join(devLocation, checkMMCDevName)
	if _, err := os.Stat(devPath); err != nil {
		return nil, err
	}
	mmcPath, err := exec.LookPath("mmc")
	if err != nil {
		return nil, err
	}
	stdout, err := exec.Command(mmcPath, "gen_cmd", "read", devPath, fmt.Sprintf("0x%X", sendCMD56Arg)).Output()
	if err != nil {
		return nil, err
	}
	res, err := parseCMD56(stdout)
	if err != nil {
		return nil, err
	}

	return res.json(), nil
}

func main() {
	tokenBytes, err := os.ReadFile("secret.psk")
	if err != nil {
		log.Fatal(err)
	}
	tokenString := "Bearer " + (*(*string)(unsafe.Pointer(&tokenBytes)))[:len(tokenBytes)-1]
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		headerToken := req.Header.Get("Authorization")
		if headerToken != tokenString {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
			return
		}
		res, err := response()
		if err != nil {
			log.Print(err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal Server Error"))
			return
		}
	        w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(res)
	})
	err = http.ListenAndServeTLS(":8443", "server.crt", "key.pem", nil)
	log.Fatal(err)
}
