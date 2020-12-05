package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newFakePod(podId string, status string) Pod_json {

	return Pod_json{
		Metadata: Metadata_json{Name: fmt.Sprintf("pod-test-%v", podId)},
		Spec:     Spec_json{NodeName: fmt.Sprintf("node-%v", podId)},
		Status: Status_json{
			HostIP: fmt.Sprintf("nip-%v", podId),
			PodIP:  fmt.Sprintf("%v", podId),
			phase:  status}}
}

func RemoveIndex(s []Pod_json, index int) []Pod_json {
	return append(s[:index], s[index+1:]...)
}

var lastFakePod int = 1
//var pingCount   int = 0
//var pingerCount int = 0


func fakeGetPods() string {

	ips, err := getUsedIPs()

	if err != nil {
		panic(err)
	}

	if len(ips) == 0 {
		panic("no host ips found for tests")
	}

	var js Pods_json = Pods_json{Pods: []Pod_json{
		newFakePod(ips[0], "Running")}}

	for i := lastFakePod; i < lastFakePod+5; i++ {
		var status = fmt.Sprintf("Running")
		if i%5 == 0 {
			status = "Stopped"
		}

		js.Pods = append(js.Pods, newFakePod(strconv.Itoa(i), status))
	}

	lastFakePod = lastFakePod + rand.Intn(3)

	//fmt.Println(fmt.Sprintf("len=%v %v",len(js.Pods),js.Pods))
	s, _ := json.Marshal(js)
	return string(s)
}

type fakeReader struct{}

func (fReader *fakeReader) Read(p []byte) (int, error) {
	result := "success\n"
	if rand.Intn(3) == 0 {
		result = "fail\n"
	}

	buf := []byte(fmt.Sprintf(result))
	copy(p, buf)

	//time.Sleep(10 * time.Millisecond)
	//pingCount++
	return len(buf), nil
}

func fakeRun(cmd string, args []string) (*bufio.Scanner, error) {

	var scanner *bufio.Scanner

	if cmd == "ping" {
		scanner = bufio.NewScanner(&fakeReader{})
	} else {
		reader := strings.NewReader(fakeGetPods())
		scanner = bufio.NewScanner(reader)
	}
	
	//pingerCount++
	return scanner, nil
}

func TestRace(test *testing.T) {

	output := make(chan PingRecord)

	go func() {
		newPingersPool("test", output, 1*time.Nanosecond, fakeRun)
	}()

	startTime := time.Now()
	for {
		//time.Sleep(10 * time.Millisecond)
		record, ok := <-output
		if !ok {
			break
		}
		fmt.Println(record.toString())
		
		
		elapsed := time.Since(startTime)
		if elapsed.Milliseconds() > 12000 {
			break
		}
	}
	
	test.Logf(fmt.Sprintf("faked pods = %v", lastFakePod))
}

