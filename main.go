package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"k8s.io/klog/v2"
)

//
// TODO(sbw): change to "bootnode" and "geth" when ready to deploy.
//
var bootnodeFile = "./bin/bootnode"
var gethFile = "./bin/geth"

func main() {
	klog.InitFlags(nil)

	var kubeconfig string
	var privateKeyPath string
	var keystorePath string
	
	flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	flag.StringVar(&privateKeyPath, "private-key", "", "path to write private key to")
	flag.StringVar(&privateKeyPath, "keystore", "", "path to keystore")
	flag.Parse()

	if privateKeyPath == "" {
		klog.Fatalf("-private-key required")
	}
	if keystorePath == "" {
		klog.Fatalf("-keystore required")
	}
	
	//
	// Create kubernetes client
	//
	if false {
		clientset, err := newClientset(kubeconfig)
		if err != nil {
			klog.Fatalf("Failed to connect to cluster: %v", err)
		}

		fmt.Printf("clientset=%v\n", clientset)
	}
		
	bootnodeExecPath, err := exec.LookPath(bootnodeFile)
	if err != nil {
		klog.Fatalf("Failed to find bootnode: %v", err)
	}
	gethExecPath, err := exec.LookPath(gethFile)
	if err != nil {
		klog.Fatalf("Failed to find geth: %v", err)
	}

	cmdGenkey := exec.Cmd{
		Path: bootnodeExecPath,
		Args: []string{bootnodeExecPath, "-genkey", privateKeyPath},
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	if err := cmdGenkey.Run(); err != nil {
		klog.Fatalf("'bootnode -genkey' failed: %v", err)
	}
	klog.Infof("Generated private key %s", privateKeyPath)	

	//
	// TODO(sbw): generate a password file with random password.
	//
	passwordPath := ""

	gethImportCmd := exec.Cmd{
		Path: gethExecPath,
		Args: []string{
			gethExecPath,
			"account", "import",
			"--keystore", keystorePath,
			"--password", passwordPath,
			privateKeyPath,
		},
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	if err := gethImportCmd.Run(); err != nil {
		klog.Fatalf("'geth account import' failed: %v", err)
	}
	klog.Infof("Imported private key %s", privateKeyPath)	

	//
	// TODO(sbw): publish enodes so they can be discovered by the proxy-informer.
	// Given the private key file and IP, bootnode will generate the enode.
	//
}

func newClientset(filename string) (*kubernetes.Clientset, error) {
	config, err := getConfig(filename)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

func getConfig(cfg string) (*rest.Config, error) {
	if cfg == "" {
		return rest.InClusterConfig()
	}
	return clientcmd.BuildConfigFromFlags("", cfg)
}
