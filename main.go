package main

import (
	"bytes"
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
	var passwordPath string
	var internalAddress string
	var externalAddress string

	flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	flag.StringVar(&privateKeyPath, "private-key", "", "path to write private key to")
	flag.StringVar(&keystorePath, "keystore", "", "path to keystore")
	flag.StringVar(&passwordPath, "password", "", "path to write password file to")
	flag.StringVar(&internalAddress, "internal-address", "", "internal proxy address")
	flag.StringVar(&externalAddress, "external-address", "", "external proxy address")
	flag.Parse()

	if privateKeyPath == "" {
		klog.Fatalf("-private-key required")
	}
	if keystorePath == "" {
		klog.Fatalf("-keystore required")
	}
	if passwordPath == "" {
		klog.Fatalf("-password required")
	}
	if internalAddress == "" {
		klog.Fatalf("-internal-address required")
	}
	if externalAddress == "" {
		klog.Fatalf("-external-address required")
	}

	bootnodeExecPath, err := exec.LookPath(bootnodeFile)
	if err != nil {
		klog.Fatalf("Failed to find bootnode: %v", err)
	}
	gethExecPath, err := exec.LookPath(gethFile)
	if err != nil {
		klog.Fatalf("Failed to find geth: %v", err)
	}

	//
	// Generate a new throw-away key that we plan on passing to the Proxy.
	//
	cmdGenPrivKey := exec.Cmd{
		Path:   bootnodeExecPath,
		Args:   []string{bootnodeExecPath, "-genkey", privateKeyPath},
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	if err := cmdGenPrivKey.Run(); err != nil {
		klog.Fatalf("'bootnode -genkey' failed: %v", err)
	}
	klog.Infof("Generated private key at: %s", privateKeyPath)

	//
	// Use bootnode to compute the public key from the private key
	//
	publicKeyBuffer := new(bytes.Buffer)
	cmdGenPubKey := exec.Cmd{
		Path:   bootnodeExecPath,
		Args:   []string{bootnodeExecPath, "-writeaddress", "-nodekey", privateKeyPath},
		Stdout: publicKeyBuffer,
		Stderr: os.Stderr,
	}
	if err := cmdGenPubKey.Run(); err != nil {
		klog.Fatalf("'bootnode -writeaddress -nodekey' failed: %v", err)
	}
	publicKey := publicKeyBuffer.String()
	klog.Infof("Generated public key: %s", publicKey)
	return

	//
	// TODO(sbw): generate a password file with random password and write to passwordPath
	//
	password := ""
	passwordFile, err := os.Create(passwordPath)
	if err != nil {
		klog.Fatalf("Unable to create password file: %v", err)
	}
	if _, err := passwordFile.WriteString(password); err != nil {
		klog.Fatalf("Unable to write password file: %v", err)
	}

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
	// TODO(sbw): extract *account address* from the filename in the keystore
	// or the JSON blob in the file contents.
	//
	accountAddress := ""
	accountAddressPath := ""
	klog.Infof("Extracted account address %s from %s", accountAddress, accountAddressPath)

	//
	// TODO(sbw): publish enodes (e.g., to a ConfigMap) so the proxy-informer
	// will discover them.
	//
	// Given the private key file and IP, bootnode will generate the enode.
	//
	internalEnode := ""
	externalEnode := ""

	//
	// Create kubernetes client
	//
	if false {
		clientset, err := newClientset(kubeconfig)
		if err != nil {
			klog.Fatalf("Failed to connect to cluster: %v", err)
		}

		fmt.Printf("clientset=%v\n", clientset)

		publishEnodes(clientset, internalEnode, externalEnode)
	}
}

func publishEnodes(clientset *kubernetes.Clientset, internalEnode string, externalEnode string) (*kubernetes.Clientset, error) {
	return nil, nil
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
