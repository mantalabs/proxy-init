package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sethvargo/go-password/password"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

// These should match with the values in proxy-informer.
var internalEnodeKey = "proxy.mantalabs.com/internal-enode-url"
var externalEnodeKey = "proxy.mantalabs.com/external-enode-url"

var bootnodeFile = "bootnode"
var gethFile = "geth"

func generatePassword() string {
	res, err := password.Generate(64, 10, 10, false, false)
	if err != nil {
		klog.Fatalf("Unable to generate password: %v", err)
	}
	return res
}

func main() {
	klog.InitFlags(nil)

	var kubeconfig string
	var privateKeyPath string
	var accountAddressPath string
	var internalAddress string
	var externalAddress string
	var podName string
	var podNamespace string

	flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	flag.StringVar(&privateKeyPath, "private-key", "", "path to write private key to")
	flag.StringVar(&accountAddressPath, "account-address", "", "path to write account address to")
	flag.StringVar(&podNamespace, "pod-namespace", "default", "namespace of Pod to annotate")
	flag.StringVar(&podName, "pod-name", "", "name of Pod to annotate")
	flag.StringVar(&internalAddress, "internal-address", "", "internal proxy address")
	flag.StringVar(&externalAddress, "external-address", "", "external proxy address")
	flag.Parse()

	if privateKeyPath == "" {
		klog.Fatalf("-private-key required")
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
	if podName == "" {
		klog.Fatalf("-pod-name required")
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
	publicKey := strings.TrimSpace(publicKeyBuffer.String())
	klog.Infof("Generated public key: %s", publicKey)

	//
	// Generate a password file with random password and write to passwordPath
	//
	passwordFile, err := ioutil.TempFile("/dev/shm", "password")
	if err != nil {
		klog.Fatalf("Unable to create password file in /dev/shm: %v", err)
	}

	password := generatePassword()
	passwordPath := passwordFile.Name()
	err = ioutil.WriteFile(passwordPath, []byte(password), 0600)
	if err != nil {
		klog.Fatalf("Unable to create password file: %v", err)
	}
	defer os.Remove(passwordPath)

	keystorePath, err := ioutil.TempDir("/dev/shm", "keystore")
	if err != nil {
		klog.Fatalf("Unable to create keystore in /dev/shm: %v", err)
	}
	defer os.RemoveAll(keystorePath)

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
	keystoreJSONPath := ""
	matchCount := 0
	matches, err := filepath.Glob(keystorePath + "/*")
	if err != nil {
		klog.Fatalf("Couldn't expand glob: %s/*: %v", keystorePath, err)
	}
	for _, match := range matches {
		keystoreJSONPath = match
		matchCount++
	}
	if matchCount != 1 {
		klog.Fatalf("Expected exactly 1 file in keystore; got: %d", matchCount)
	}
	// (2) Load the JSON blob to extract the address.
	keystoreJSONContent, err := ioutil.ReadFile(keystoreJSONPath)
	if err != nil {
		klog.Fatalf("Couldn't read keystore file %s: %v", keystoreJSONPath, err)
	}

	var keystore map[string]interface{}
	err = json.Unmarshal(keystoreJSONContent, &keystore)
	if err != nil {
		klog.Fatalf("Couldn't unmarshal keystore data: %v", err)
	}

	accountAddress := keystore["address"]
	klog.Infof("Extracted account address from keytore: %s", accountAddress)
	// (3) Write the address to he requested path
	// #nosec G306
	err = ioutil.WriteFile(accountAddressPath, []byte(accountAddress.(string)), 0644)
	if err != nil {
		klog.Fatalf("Couldn't write account address to %s: %v", accountAddressPath, err)
	}
	klog.Infof("Wrote account address to %s", accountAddressPath)

	//
	// Given the private key file and IP, bootnode will generate the enode.
	//
	internalEnode := fmt.Sprintf("enode://%s@%s", publicKey, internalAddress)
	externalEnode := fmt.Sprintf("enode://%s@%s", publicKey, externalAddress)

	//
	// Create kubernetes client
	//
	clientset, err := newClientset(kubeconfig)
	if err != nil {
		klog.Fatalf("Failed to connect to cluster: %v", err)
	}

	//
	// Publish enodes so that proxy-informers can discover the Proxy.
	//
	if err := publishEnodes(clientset, podNamespace, podName, internalEnode, externalEnode); err != nil {
		klog.Fatalf("Failed to publish enodes: %v", err)
	}
}

func publishEnodes(clientset *kubernetes.Clientset, podNamespace, podName, internalEnode, externalEnode string) error {
	patch := fmt.Sprintf(
		`{"metadata":{"annotations": {"%s":"%s", "%s":"%s"}}}`,
		internalEnodeKey,
		internalEnode,
		externalEnodeKey,
		externalEnode)
	_, err := clientset.CoreV1().Pods(podNamespace).Patch(
		context.TODO(),
		podName,
		types.StrategicMergePatchType,
		[]byte(patch),
		metav1.PatchOptions{})
	return err
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
