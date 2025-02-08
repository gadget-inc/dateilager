//go:build integration
// +build integration

package test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type K3DTestSuite struct {
	suite.Suite
	clientset *kubernetes.Clientset
	namespace string
}

func (s *K3DTestSuite) SetupSuite() {
	_, err := exec.LookPath("orb")
	if err != nil {
		s.T().Skip("orb not found in PATH")
	}

	ctx := context.Background()

	// Get kubernetes client
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(s.T(), err)

	// Set the context to orbstack
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = kubeconfig
	configOverrides := &clientcmd.ConfigOverrides{
		CurrentContext: "orbstack", // orbstack
	}

	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err = kubeConfig.ClientConfig()
	require.NoError(s.T(), err)

	s.clientset, err = kubernetes.NewForConfig(config)
	require.NoError(s.T(), err)

	s.namespace = createNamespaceName()

	_, err = s.clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: s.namespace,
		},
	}, metav1.CreateOptions{})
	require.NoError(s.T(), err)

	// Wait for namespace to be ready
	for i := 0; i < 30; i++ {
		_, err = s.clientset.CoreV1().Namespaces().Get(ctx, s.namespace, metav1.GetOptions{})
		if err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	require.NoError(s.T(), err, "timeout waiting for default namespace")

	// Create default ServiceAccount if it doesn't exist
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: s.namespace,
		},
	}
	_, err = s.clientset.CoreV1().ServiceAccounts(s.namespace).Create(ctx, sa, metav1.CreateOptions{})
	require.NoError(s.T(), err, "failed to create default ServiceAccount")

	s.T().Log("Default ServiceAccount created successfully")
	ctx = context.Background()
	for i := 0; i < 30; i++ {
		_, err = s.clientset.CoreV1().ServiceAccounts(s.namespace).Get(ctx, "default", metav1.GetOptions{})
		if err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	require.NoError(s.T(), err, "timeout waiting for default ServiceAccount")
}

func (s *K3DTestSuite) TearDownSuite() {
	if s.clientset != nil {
		cleanup := exec.Command("kubectl", "delete", "all", "--all", "-n", s.namespace)
		if err := cleanup.Run(); err != nil {
			s.T().Logf("Failed to cleanup namespace: %v", err)
		}

		deleteNamespace := exec.Command("kubectl", "delete", "namespace", s.namespace)
		if err := deleteNamespace.Run(); err != nil {
			s.T().Logf("Failed to delete namespace: %v", err)
		}
	}
}

func (s *K3DTestSuite) TestCachedCSIDriver() {
	// Apply CSI driver YAML
	applyCSIDriver := exec.Command("kubectl", "apply", "-n", s.namespace, "-f", "../test/k8s-local/cached-csi.yaml")
	require.NoError(s.T(), applyCSIDriver.Run(), "failed to apply CSI driver")

	// Apply daemon set YAML
	applyDaemonSet := exec.Command("kubectl", "apply", "-n", s.namespace, "-f", "../test/k8s-local/cached-daemon.yaml")
	applyDaemonSet.Stdout = os.Stdout
	applyDaemonSet.Stderr = os.Stderr
	require.NoError(s.T(), applyDaemonSet.Run(), "failed to apply daemon set")

	ready := false
	// Wait for daemon set to be ready
	for i := 0; i < 60; i++ {
		ds, err := s.clientset.AppsV1().DaemonSets(s.namespace).Get(context.Background(), "dateilager-csi-cached", metav1.GetOptions{})
		if err == nil && ds.Status.NumberReady == ds.Status.DesiredNumberScheduled {
			ready = true
			break
		}
		time.Sleep(time.Second)
	}
	require.True(s.T(), ready, "daemon set failed to become ready")
	dsPods, err := s.clientset.CoreV1().Pods(s.namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: "app=dateilager-csi-cached", // This should match the labels in your DaemonSet spec
	})
	require.NoError(s.T(), err, "failed to list pods")
	require.NotEmpty(s.T(), dsPods.Items, "no pods found for daemon set")

	dsPod := dsPods.Items[0]
	_, _, err = s.execInPodKubectl(dsPod.Name, []string{"ls", "/var/lib/kubelet/dateilager_cache"})
	require.NoError(s.T(), err, "failed to get pods")

	// Create some files in the cache
	_, _, err = s.execInPodKubectl(dsPod.Name, []string{"mkdir", "-p", "/var/lib/kubelet/dateilager_cache/dl_cache/objects/v1/m1/node_modules", "/var/lib/kubelet/dateilager_cache/dl_cache/objects/v2/m2/node_modules"})
	require.NoError(s.T(), err, "failed to create files in cache")

	_, _, err = s.execInPodKubectl(dsPod.Name, []string{"echo", "foo", ">", "/var/lib/kubelet/dateilager_cache/dl_cache/objects/v1/m1/node_modules/index.js"})
	require.NoError(s.T(), err, "failed to create file in cache")

	_, _, err = s.execInPodKubectl(dsPod.Name, []string{"echo", "bar", ">", "/var/lib/kubelet/dateilager_cache/dl_cache/objects/v2/m2/node_modules/index.js"})
	require.NoError(s.T(), err, "failed to create file in cache")

}

func (s *K3DTestSuite) TestDeployPodUsingCachedMount() {
	yamlFile, err := os.ReadFile("../test/k8s-local/busy-box-debug.yaml")
	require.NoError(s.T(), err, "failed to read yaml file")

	pod := corev1.Pod{}
	reader := bytes.NewReader(yamlFile)
	b := make([]byte, reader.Size())
	_, err = reader.Read(b)
	require.NoError(s.T(), err, "failed to read busybox pod spec")

	err = k8syaml.Unmarshal(b, &pod)
	require.NoError(s.T(), err, "failed to decode yaml")

	// Modify the pod name
	pod.Name = "cached-pod1"

	// Create the pod
	_, err = s.clientset.CoreV1().Pods(s.namespace).Create(
		context.Background(),
		&pod,
		metav1.CreateOptions{},
	)
	require.NoError(s.T(), err, "failed to create pod")

	// Wait for pod to be ready
	err = waitForPod(s.clientset, s.namespace, pod.Name)
	require.NoError(s.T(), err, "pod failed to become ready")

	// Check if the files are in the cache
	var stdout string
	stdout, _, err = s.execInPodKubectl(pod.Name, []string{"ls", "/gadget/dl_cache/objects"})
	require.NoError(s.T(), err, "failed to list files in gadget dl_cache")
	require.Contains(s.T(), stdout, "v1")
	require.Contains(s.T(), stdout, "v2")

	_, _, err = s.execInPodKubectl(pod.Name, []string{"mkdir", "-p", "/gadget/app/node_modules/m1", "/gadget/app/node_modules/m2"})
	_, _, err = s.execInPodKubectl(pod.Name, []string{"ln", "-s", "/gadget/dl_cache/objects/v1/m1/node_modules/index.js", "/gadget/app/node_modules/m1/index.js"})
	require.NoError(s.T(), err, "failed to link m1")
	_, _, err = s.execInPodKubectl(pod.Name, []string{"ln", "-s", "/gadget/dl_cache/objects/v2/m2/node_modules/index.js", "/gadget/app/node_modules/m2/index.js"})
	require.NoError(s.T(), err, "failed to link m2")
	s.T().Log("Busybox pod deployed successfully")
}

func (s *K3DTestSuite) execInPodKubectl(podName string, command []string) (string, string, error) {
	args := append([]string{"exec", "-n", s.namespace, podName, "--"}, command...)
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("kubectl", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return "", "", err
	}

	return stdout.String(), stderr.String(), nil
}

// Helper function to wait for pod readiness
func waitForPod(clientset *kubernetes.Clientset, namespace, name string) error {
	return wait.PollImmediate(time.Second, time.Minute, func() (bool, error) {
		pod, err := clientset.CoreV1().Pods(namespace).Get(
			context.Background(),
			name,
			metav1.GetOptions{},
		)
		if err != nil {
			return false, err
		}
		return pod.Status.Phase == corev1.PodRunning, nil
	})
}

func createNamespaceName() string {
	name := fmt.Sprintf("dateilager-local-%s", uuid.New().String()[:4])
	name = strings.ReplaceAll(name, "_", "-")
	return name
}

func TestK3DSuite(t *testing.T) {
	suite.Run(t, new(K3DTestSuite))
}
