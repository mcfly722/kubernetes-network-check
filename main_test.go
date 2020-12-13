package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"
	"strconv"


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

var pingCount_mu sync.Mutex
var pingCount int = 0

var pingerCount_mu sync.Mutex
var pingerCount int = 0

var changesOffset_mu sync.Mutex
var changesOffset int = 1 // this counter

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

func fakeConfigChanger(k8s *k8s,namespace string,podPrefix string,ip string) {

	for {
		changesOffset_mu.Lock()
		changesOffset = changesOffset + rand.Intn(3)
		changesOffset_mu.Unlock()

		pods,err := getPods(k8s, namespace, podPrefix)
		if err != nil {
			panic (err)
		}
		if len(pods) > 3 + rand.Intn(5) {
			for i:=0;i<rand.Intn(5);i++ {
				deleteFirst: for key, pod := range pods {
					if pod.PodIP != ip {
						k8s.clientset.CoreV1().Pods(namespace).Delete(pod.PodName, &metav1.DeleteOptions{})
						delete(pods, key)
						fmt.Println(fmt.Sprintf("%3d - %v",len(pods), pod.PodName))
						break deleteFirst
					}
				}
			}
		} else {
			changesOffset_mu.Lock()
			fakePod := newFakePod(strconv.Itoa(changesOffset), "Running", namespace)
			changesOffset_mu.Unlock()
			k8s.clientset.CoreV1().Pods(namespace).Create(fakePod)
			fmt.Println(fmt.Sprintf("%3d + %v",len(pods), fakePod.Name))
		}
		time.Sleep(30 * time.Millisecond)
		runtime.Gosched()
	}
}

func TestRace(test *testing.T) {
	
	ips, err := getUsedIPs()

	if err != nil {
		panic(err)
	}
	if len(ips) < 1 {
		panic("there are no IPs for tests")
	}


	k8s := newFakeK8s()

	output := make(chan PingRecord)

	baseFakePod := newFakePod(ips[0], "Running", "monitoring")
	k8s.clientset.CoreV1().Pods("monitoring").Create(baseFakePod)

	go func() {
		fakeConfigChanger(k8s, "monitoring","kubernetes-network-check",ips[0])
	}()

	go func() {
		newPingersPool(k8s, "monitoring", "kubernetes-network-check", output, 1*time.Second, 1, fakeRun)
	}()

	startTime := time.Now()
testing:
	for {
		select {
		case record := <-output:
			//fmt.Println(record.toString())
			fmt.Println(fmt.Sprintf("    . %v -> %v",record.Source.PodName,record.Destination.PodName));
		default:
			elapsed := time.Since(startTime)
			if elapsed.Milliseconds() > 3000 {
				break testing
			}
		}

		time.Sleep(3   * time.Millisecond)
	}

	changesOffset_mu.Lock()
	pingCount_mu.Lock()
	pingerCount_mu.Lock()
	
	fmt.Println(fmt.Sprintf("changesOffset = %v\nping = %v\npingers = %v", changesOffset, pingCount, pingerCount))

	changesOffset_mu.Unlock()
	pingCount_mu.Unlock()
	pingerCount_mu.Unlock()
}
