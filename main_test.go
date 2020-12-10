package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newFakeK8s() *k8s {
	client := k8s{}
	client.clientset = fake.NewSimpleClientset()
	return &client
}

func newFakePod(podId string, status v1.PodPhase, namespace string) *v1.Pod {

	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("kubernetes-network-check-%v", podId),
			Namespace: namespace,
		},
		Spec: v1.PodSpec{NodeName: fmt.Sprintf("node-%v", podId)},
		Status: v1.PodStatus{
			HostIP: fmt.Sprintf("nip-%v", podId),
			PodIP:  fmt.Sprintf("%v", podId),
			Phase:  status},
	}
}

var lastFakePod int = 1

var pingCount_mu sync.Mutex
var pingCount int = 0

var pingerCount_mu sync.Mutex
var pingerCount int = 0

/*
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
*/

type fakeReader struct{}

func (fReader *fakeReader) Read(p []byte) (int, error) {
	result := "success\n"
	if rand.Intn(3) == 0 {
		result = "fail\n"
	}

	buf := []byte(fmt.Sprintf(result))
	copy(p, buf)

	//time.Sleep(10 * time.Millisecond)

	pingCount_mu.Lock()
	pingCount++
	pingCount_mu.Unlock()

	return len(buf), nil
}

func fakeRun(cmd string, args []string) (*bufio.Scanner, error) {
	scanner := bufio.NewScanner(&fakeReader{})

	pingerCount_mu.Lock()
	pingerCount++
	pingerCount_mu.Unlock()

	runtime.Gosched()

	return scanner, nil
}

func RemoveIndex(s []v1.Pod, index int) []v1.Pod {
	return append(s[:index], s[index+1:]...)
}

var changesOffset int = 1 // this counter

func fakeConfigChanger(k8s *k8s) {

	for {
		changesOffset = changesOffset + rand.Intn(3)

		//test.Logf(fmt.Sprintf("changeOffset=%v", changeOffset))

		time.Sleep(10 * time.Millisecond)
		runtime.Gosched()
	}
}

func TestRace(test *testing.T) {

	k8s := newFakeK8s()

	output := make(chan PingRecord)

	go func() {
		fakeConfigChanger(k8s)
	}()

	go func() {
		newPingersPool(k8s, "monitoring", "kubernetes-network-check", output, 1*time.Second, 1, fakeRun)
	}()

	startTime := time.Now()
testing:
	for {
		select {
		case record := <-output:
			fmt.Println(record.toString())
		default:
			elapsed := time.Since(startTime)
			if elapsed.Milliseconds() > 3000 {
				break testing
			}
		}

		time.Sleep(10 * time.Millisecond)
	}

	test.Logf(fmt.Sprintf("changesOffset = %v\nlast fake pod = %v\nping = %v\npingers = %v", changesOffset, lastFakePod, pingCount, pingerCount))
}
