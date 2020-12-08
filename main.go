package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"time"
)

type Spec_json struct {
	NodeName string `json:"nodeName"`
}

type Status_json struct {
	HostIP string `json:"hostIP"`
	PodIP  string `json:"podIP"`
	Phase  string `json:"phase"`
}

type Metadata_json struct {
	Name string `json:"name"`
}

type Pod_json struct {
	Metadata Metadata_json `json:"metadata"`
	Spec     Spec_json     `json:"spec"`
	Status   Status_json   `json:"status"`
}

type Pods_json struct {
	Pods []Pod_json `json:"items"`
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

func getPods(filter string, run func(cmd string, arg []string) (*bufio.Scanner, error)) (map[string]Pod, error) {
	pods := make(map[string]Pod)

	args := []string{"get", "pods", "-A", "--alsologtostderr","-o","json"}

	scanner, err := run("/kubectl", args)
	if err != nil {
		return nil, err
	}

	var cmdOut = ""
	for scanner.Scan() {
		cmdOut = cmdOut + scanner.Text()
	}

	var js Pods_json
	err = json.Unmarshal([]byte(cmdOut), &js)

	if err != nil {
		return nil, err
	}

	for i := 0; i < len(js.Pods); i++ {
		match, _ := regexp.MatchString(filter, js.Pods[i].Metadata.Name)
		//fmt.Println(fmt.Sprintf("match: %v - %v - %v", js.Pods[i].Metadata.Name, filter, match))
		if match {
			if js.Pods[i].Status.Phase == "Running" {
				pod := Pod{
					PodName:  js.Pods[i].Metadata.Name,
					PodIP:    js.Pods[i].Status.PodIP,
					HostName: js.Pods[i].Spec.NodeName,
					HostIP:   js.Pods[i].Status.HostIP}

				pods[pod.hash()] = pod
			}
		}
	}

	return pods, nil
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
//	commandErr, _ := command.StderrPipe()
	output_scanner:= bufio.NewScanner(commandOut)
//	err_scanner   := bufio.NewScanner(commandErr)
	err := command.Start()

/*
	if err == nil { // out errStdout even if there are no execution errors
		for err_scanner.Scan() {
			fmt.Println(err_scanner.Text())
		}
	}
*/

	return output_scanner, err
}

func newPinger(source Pod, destination Pod, output chan PingRecord, run func(cmd string, arg []string) (*bufio.Scanner, error)) (*Pinger, error) {

	pinger := Pinger{
		Done: make(chan struct{}),
	}

	go func() {
		args := []string{destination.PodIP, "-i", "5"}
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
							record := PingRecord{
								Source:      source,
								Destination: destination,
								Message:     text}
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

func newPingersPool(filter string, output chan PingRecord, configRefreshInterval time.Duration, run func(cmd string, arg []string) (*bufio.Scanner, error)) {

	pingers := map[string]*Pinger{}

	ips, err := getUsedIPs()
	if err != nil {
		//time.Sleep(2 * time.Second) // do not restart pod immediately
		panic(err)
	}

	fmt.Println(fmt.Sprintf("used ips: %v", ips))
	
	for {

		//fmt.Println(fmt.Sprintf("UsedIps:%v", strings.Join(ips,",\n")));

		pods, err := getPods(filter, run)
		if err != nil {
			//time.Sleep(2 * time.Second) // do not restart pod immediately
			fmt.Println(fmt.Sprintf("error: %v", err));
			//panic(err)
		} else {
			fmt.Println(fmt.Sprintf("used pods: %v", pods));

			// search current pod
			for _, sourcePod := range pods {

				if contains(ips, sourcePod.PodIP) {

					s,_ := json.Marshal(sourcePod)
					fmt.Println(fmt.Sprintf("current pod: %v", string(s)));

					// add new pingers
					for key, pod := range pods {
						_, exist := pingers[key]
						if !exist {
							pinger, _ := newPinger(sourcePod, pod, output, run)
							pingers[key] = pinger
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

	output := make(chan PingRecord)

	go func() {
		newPingersPool("kubernetes-network-check-", output, 30 * time.Second, run)
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
