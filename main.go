package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type k8s struct {
	clientset kubernetes.Interface
}

func newK8s() (*k8s, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	client := k8s{}

	client.clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &client, nil
}

type Pod struct {
	PodName  string
	PodIP    string
	HostName string
	HostIP   string
}

func (pod Pod) hash() string {
	s, _ := json.Marshal(pod)
	return string(s)
}

type PingRecord struct {
	Source      Pod
	Destination Pod
	Message     string
	Elapsed_ms  float64
	Success     bool
}

func (record *PingRecord) toString() string {
	json, _ := json.Marshal(record)
	return string(json)

	//return fmt.Sprintf("%15s->%15s  : %s", record.Source.PodIP, record.Destination.PodIP, record.Message)
}

func newPod(podName string, podIP string, hostName string, hostIP string) *Pod {
	return &Pod{
		PodName:  podName,
		PodIP:    podIP,
		HostName: hostName,
		HostIP:   hostIP,
	}
}

func getUsedIPs() ([]string, error) {
	ips := []string{}

	ifaces, err := net.Interfaces()

	if err != nil {
		return nil, err
	}

	for _, i := range ifaces {
		addrs, err := i.Addrs()

		if err != nil {
			return nil, err
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			ips = append(ips, fmt.Sprintf("%v", ip))
		}
	}

	return ips, nil
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

func getPods(k8s *k8s, namespace string, podPrefix string) (map[string]Pod, error) {
	result := make(map[string]Pod)

	pods, err := k8s.clientset.CoreV1().Pods(namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, pod := range pods.Items {
		if strings.HasPrefix(strings.ToUpper(pod.GetName()), strings.ToUpper(podPrefix)) {

			if pod.Status.Phase == "Running" {
				pod := Pod{
					PodName:  pod.GetName(),
					PodIP:    pod.Status.PodIP,
					HostName: pod.Spec.NodeName,
					HostIP:   pod.Status.HostIP}

				result[pod.hash()] = pod
			}
		}
	}

	return result, nil

}

type Pinger struct {
	Done chan struct{}
}

func (pinger *Pinger) Destroy() {
	close(pinger.Done)
}

func run(cmd string, args []string) (*bufio.Scanner, error) {
	fmt.Println(fmt.Sprintf("exec: %v %v", cmd, args))
	command := exec.Command(cmd, args...)
	commandOut, _ := command.StdoutPipe()
	output_scanner := bufio.NewScanner(commandOut)
	err := command.Start()

	return output_scanner, err
}

func getParams(regEx, url string) (paramsMap map[string]string) {

	var compRegEx = regexp.MustCompile(regEx)
	match := compRegEx.FindStringSubmatch(url)

	paramsMap = make(map[string]string)
	for i, name := range compRegEx.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}
	return
}

func newPinger(source Pod, destination Pod, intervalSec int, output chan PingRecord, run func(cmd string, arg []string) (*bufio.Scanner, error)) (*Pinger, error) {

	pinger := Pinger{
		Done: make(chan struct{}),
	}

	go func() {
		args := []string{destination.PodIP, "-i", strconv.Itoa(intervalSec)}
		scanner, err := run("/bin/ping", args)

		if err != nil {
			fmt.Println(fmt.Sprintf("pinger error %v", err))
		} else {
			fmt.Println(fmt.Sprintf("pinger for '%s' started", destination.PodName))

		working:
			for {
				select {
				case <-pinger.Done:
					break working
				default:
					if scanner.Scan() {
						text := scanner.Text()
						if len(text) > 0 {

							var elapsed float64 = 0

							m := getParams(`.+\stime=(?P<elapsed>[-+]?[0-9]*\.?[0-9]*)\sms$`, text)
							if _, ok := m["elapsed"]; ok {

								e, err := strconv.ParseFloat(m["elapsed"], 64)
								if err == nil {
									elapsed = e
								}
							}
							
							successMatched, _:= regexp.Match(`bytes from`, []byte(text))
							

							record := PingRecord{
								Source:      source,
								Destination: destination,
								Message:     text,
								Elapsed_ms:  elapsed,
								Success:     successMatched}

							output <- record
						}
					}
				}
				runtime.Gosched()
			}
			fmt.Println(fmt.Sprintf("pinger for '%v' finished", destination.PodName))
		}
	}()

	return &pinger, nil
}

func newPingersPool(k8s *k8s, namespace string, podsPrefix string, output chan PingRecord, configRefreshInterval time.Duration, pingIntervalSec int, run func(cmd string, arg []string) (*bufio.Scanner, error)) {

	pingers := map[string]*Pinger{}

	ips, err := getUsedIPs()
	if err != nil {
		//time.Sleep(2 * time.Second) // do not restart pod immediately
		panic(err)
	}

	fmt.Println(fmt.Sprintf("used ips: %v", ips))

	for {

		//fmt.Println(fmt.Sprintf("UsedIps:%v", strings.Join(ips,",\n")));

		pods, err := getPods(k8s, namespace, podsPrefix)
		if err != nil {
			//time.Sleep(2 * time.Second) // do not restart pod immediately
			fmt.Println(fmt.Sprintf("error: %v", err))
			//panic(err)
		} else {
			fmt.Println(fmt.Sprintf("used pods: %v", pods))

			// search current pod
			for _, sourcePod := range pods {

				if contains(ips, sourcePod.PodIP) {

					s, _ := json.Marshal(sourcePod)
					fmt.Println(fmt.Sprintf("current pod: %v", string(s)))

					// add new pingers
					for key, pod := range pods {
						_, exist := pingers[key]
						if !exist {
							if !contains(ips, pod.PodIP) { // do not ping itself
								pinger, _ := newPinger(sourcePod, pod, pingIntervalSec, output, run)
								pingers[key] = pinger
							}
						}
					}

					// delete unused pingers
					for key := range pingers {
						_, exists := pods[key]
						if !exists {
							pingers[key].Destroy()
							delete(pingers, key)
						}
					}
				}
			}
		}
		time.Sleep(configRefreshInterval)
	}
}

func main() {
	var updateConfigSecFlag *int
	var pingIntervalSecFlag *int
	var namespaceFlag *string
	var podsPrefixFlag *string

	updateConfigSecFlag = flag.Int("updateConfigIntervalSec", 30, "interval in seconds between asking cluster for ping pods configuration")
	pingIntervalSecFlag = flag.Int("pingIntervalSec", 1, "equal ping -i parameter")
	namespaceFlag = flag.String("namespace", "monitoring", "pods namespace")
	podsPrefixFlag = flag.String("podsPrefix", "kubernetes-network-check", "pods prefix")

	flag.Parse()

	fmt.Println(fmt.Sprintf("updateConfigIntervalSec = %v", *updateConfigSecFlag))
	fmt.Println(fmt.Sprintf("pingIntervalSec = %v", *pingIntervalSecFlag))
	fmt.Println(fmt.Sprintf("namespace = %s", *namespaceFlag))
	fmt.Println(fmt.Sprintf("podsPrefix = %s", *podsPrefixFlag))

	k8s, err := newK8s()
	if err != nil {
		panic(err)
	}

	output := make(chan PingRecord)

	go func() {
		newPingersPool(k8s, *namespaceFlag, *podsPrefixFlag, output, time.Duration(*updateConfigSecFlag)*time.Second, *pingIntervalSecFlag, run)
	}()

	// write to output all records
	for {
		time.Sleep(10 * time.Millisecond)

		record, ok := <-output
		if !ok {
			break
		}

		fmt.Println(record.toString())
	}

}
