package main

import (
	"fmt"
//    "io/ioutil"
    "bufio"
    "os/exec"
	"time"
	"encoding/json"
	"net"
//	"strings"
	"regexp"
)

type Spec_json struct {
	NodeName string `json:"nodeName"`
}

type Status_json struct {
	HostIP string `json:"hostIP"`
	PodIP  string `json:"podIP"`
	phase  string `json:"phase"`
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
	s,_ := json.Marshal(pod)
	return string(s)
}

type PingRecord struct {
	Source Pod
	Destination Pod
	Message string
}

func (record *PingRecord) toString() string {
	//json, _ := json.Marshal(record)
	//return string(json)
	
	return fmt.Sprintf("%15s->%15s  : %s", record.Source.PodIP,record.Destination.PodIP, record.Message)
}

func newPod(podName string, podIP string, hostName string, hostIP string) *Pod {
	return &Pod{
		PodName : podName,
		PodIP : podIP,
		HostName : hostName,
		HostIP : hostIP,
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
			ips = append(ips, fmt.Sprintf("%v",ip))
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

func getPods(filter string) (map[string]Pod, error){
	pods := make (map[string]Pod)
	
	args := []string{"/c", "type", "kctl.json"}

	cmd := exec.Command("cmd.exe",args...)
	cmdOut, err := cmd.Output()
	
	if err != nil {
		return nil,err
	}

	var js Pods_json
	err = json.Unmarshal(cmdOut,&js)

	if err != nil {
		return nil, err
	}
	
	for i:=0;i<len(js.Pods);i++ {
		match,_ :=regexp.MatchString(podFilter, js.Pods[i].Metadata.Name)
		if match {
			pod :=Pod {
				PodName  : js.Pods[i].Metadata.Name,
				PodIP    : js.Pods[i].Status.PodIP,
				HostName : js.Pods[i].Spec.NodeName,
				HostIP   : js.Pods[i].Status.HostIP}
				
			pods[pod.hash()] = pod
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

func newPinger(source Pod, destination Pod, output chan PingRecord) (*Pinger, error) {

	pinger := Pinger {
		Done : make (chan struct{}),
	}
	
	go func() {
		
		args := []string{"-t", destination.PodIP}
		cmd := exec.Command("ping",args...)
		
		cmdOut, _ := cmd.StdoutPipe()
		scanner := bufio.NewScanner(cmdOut)
	
		err := cmd.Start()
		if err != nil {
			fmt.Println("pinger error %v", err);
		} else {
			fmt.Println(fmt.Sprintf("pinger for '%s' started", destination.PodName));
			for {
				// read all command output to channel
				for scanner.Scan() {
				
					text := scanner.Text()
					
					if len(text) > 0 {
						record := PingRecord {
							Source      : source,
							Destination : destination,
							Message     : text}
						output <- record
					}
				}
	
				// until channel will be closed
				_, ok := <-pinger.Done
				if !ok {break}
			}
			fmt.Println("pinger for '%v' finished", destination.PodName);
		}
	}()
	
	return &pinger, nil
}

func newPingersPool(filter string, output chan PingRecord) {

	pingers := map[string]*Pinger{}
	
	for {
		
		ips, err := getUsedIPs()
		if err != nil {
			time.Sleep(2 * time.Second)  // do not restart pod immediately
			panic(err)
		}
		//fmt.Println(fmt.Sprintf("UsedIps:%v", strings.Join(ips,",\n")));
		
		pods, err := getPods(filter)
		if err != nil {
			time.Sleep(2 * time.Second)  // do not restart pod immediately
			panic(err)
		}
		//fmt.Println(fmt.Sprintf("Pods:%v", pods));
		
		
		// search current pod
		for key, sourcePod := range pods {
		
			if contains(ips, pods[key].PodIP) {
				
				//s,_ := json.Marshal(sourcePod)
				//fmt.Println(fmt.Sprintf("Pod IPs [%v]\ncurrent pod=", strings.Join(ips,", "), string(s)));
				
				// add new pingers
				for key, pod := range pods {
					_, exist := pingers[key]
					if !exist {
						pinger,_ := newPinger(sourcePod, pod, output)
						pingers[key] = pinger
					}
				}
			
				// delete unused pingers
				for key,_ := range pingers {
					_, exists := pods[key]
					if !exists {
						pingers[key].Destroy()
						delete(pingers, key)
					}
				}
			}
		}
	
		fmt.Println(".");
		time.Sleep(2 * time.Second)
	}
}


const podFilter = "monitoring-busybox"

func main() {

	output := make(chan PingRecord)

	go func() {
		newPingersPool(podFilter, output)
	}()

	

	
	// write to output all records
	for {
		time.Sleep(10 * time.Millisecond)

		record, ok := <- output
		if !ok {
			break
		}
		
		fmt.Println(record.toString())
	}

}