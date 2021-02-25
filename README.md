# kubernetes-network-check

DaemonSet checks ping connectivity from each k8s node to other. Results printed in json format and then could be handled by other monitoring system (f.e. FluentD to ElasticSearch).

Example of output json (preformatted):
```
{
	"Timestamp": "2021-02-25T11:47:46Z",
	"Source": {
		"PodName": "kubernetes-network-check-p5h57",
		"PodIP": "10.42.224.0",
		"HostName": "kub-test-master3",
		"HostIP": "192.168.1.103"
	},
	"Destination": {
		"PodName": "kubernetes-network-check-9p46c",
		"PodIP": "10.42.32.0",
		"HostName": "kub-test-node2",
		"HostIP": "192.168.1.112"
	},
	"Message": "64 bytes from 10.42.32.0: seq=35011 ttl=64 time=0.362 ms",
	"Elapsed_ms": 0.362,
	"Success": true
}
```

### Deployment
Current configuration deploys DaemonSet to **monitoring** namespace. To deploy in another namespace (f.e. kube-system) you have to modify kubernetes-network-check.yaml.
```
kubectl.exe --server "<your k8s cluster address>" --token "<your token here>" --insecure-skip-tls-verify apply -f "kubernetes-network-check.yaml"
```


### Build image
```docker build --no-cache -t kubernetes-network-check:1.0 .```
