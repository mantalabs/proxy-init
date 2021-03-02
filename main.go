package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"k8s.io/klog/v2"

	"math/rand"
	"strings"
)

//
// TODO(sbw): change to "bootnode" and "geth" when ready to deploy.
//
var bootnodeFile = "./bin/bootnode"
var gethFile = "./bin/geth"

func genPass() string {
	chars := []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		"0123456789")
	length := 16
	var b strings.Builder
	for i := 0; i < length; i++ {
		b.WriteRune(chars[rand.Intn(len(chars))])
	}
	return b.String()
}

func main() {
	klog.InitFlags(nil)

	var kubeconfig string
	var privateKeyPath string
	var keystorePath string
	var passwordPath string
	var accountAddressPath string
	var internalAddress string
	var externalAddress string

	flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	flag.StringVar(&privateKeyPath, "private-key", "", "path to write private key to")
	flag.StringVar(&keystorePath, "keystore", "", "path to keystore")
	flag.StringVar(&passwordPath, "password", "", "path to write password file to")
	flag.StringVar(&accountAddressPath, "account-address", "", "path to write account address to")
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
	if accountAddressPath == "" {
		klog.Fatalf("-account-address required")
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

	//
	// Generate a password file with random password and write to passwordPath
	//
	password := genPass()
	err = ioutil.WriteFile(passwordPath, []byte(password), 0600)
	if err != nil {
		klog.Fatalf("Unable to create password file: %v", err)
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

	// Extract *account address* from the filename in the keystore
	// or the JSON blob in the file contents.
	//
	// (1) Find the keystore JSON file. The keystore file has a name like:
	//
	//   keystore-path/UTC--2021-03-01T05-17-12.173336000Z--2754599e48ca29f1998c31e7c668c33bff5e5bf2
	//
	keystoreJsonPath := ""
	matchCount := 0
	matches, err := filepath.Glob(keystorePath + "/*")
	if err != nil {
		klog.Fatalf("Couldn't expand glob: %s/*: %v", keystorePath, err)
	}
	for _, match := range matches {
		keystoreJsonPath = match
		matchCount += 1
	}
	if matchCount != 1 {
		klog.Fatalf("Expected exactly 1 file in keystore; got: %d", matchCount)
	}
	// (2) Load the JSON blob to extract the address.
	keystoreJsonContent, err := ioutil.ReadFile(keystoreJsonPath)
	if err != nil {
		klog.Fatalf("Couldn't read keystore file %s: %v", keystoreJsonPath, err)
	}
	var keystore map[string]interface{}
	err = json.Unmarshal([]byte(keystoreJsonContent), &keystore)
	if err != nil {
		klog.Fatalf("Couldn't unmarshal keystore data: %v", err)
	}
	accountAddress := keystore["address"]
	klog.Infof("Extracted account address from keytore: %s", accountAddress)
	// (3) Write the address to he requested path
	err = ioutil.WriteFile(accountAddressPath, []byte(accountAddress.(string)), 0644)
	if err != nil {
		klog.Fatalf("Couldn't write account address to %s: %v", accountAddressPath, err)
	}
	klog.Infof("Wrote account address to %s", accountAddressPath)
	return

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
