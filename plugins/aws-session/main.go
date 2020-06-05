package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

var version string

var (
	// vars used for flags
	kubeconfig string
	namespace  string

	rootCmd = &cobra.Command{
		Use:     "set-aws-session",
		Aliases: []string{"\"kubectl set-aws-session\""},
		Short:   "Add aws-session credentials to cluster",
		// 		Long: ``,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var name string
			if len(args) > 1 {
				log.Fatalf("Too many arguments. Command received %d secret-names instead of 1: %+v", len(args), strings.Join(args, ", "))
			}
			if len(args) == 0 {
				name = "aws-session-credentials"
			} else {
				name = args[0]
			}
			run(kubeconfig, namespace, name)
		},
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version number of [kubectl] set-aws-session",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("set-aws-session-%s\n", version)
		},
	}
)

// Execute executes the root command.
func Execute(v string) error {
	version = v
	return rootCmd.Execute()
}

func init() {
	if home := homedir.HomeDir(); home != "" {
		rootCmd.Flags().StringVarP(&kubeconfig, "kubecfg", "c", "", "(optional) absolute path to the kubeconfig file")
		if kubeconfig == "" {
			kubeconfig = os.Getenv("KUBECONFIG")
			if kubeconfig == "" {
				kubeconfig = filepath.Join(home, ".kube", "config")
			}
		}
	} else {
		rootCmd.Flags().StringVarP(&kubeconfig, "kubecfg", "c", "", "absolute path to the kubeconfig file")
		if kubeconfig == "" {
			kubeconfig = os.Getenv("KUBECONFIG")
		}
	}
	rootCmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "namespace to add Kubernetes creds secret")
	rootCmd.AddCommand(versionCmd)
}

func er(msg interface{}) {
	fmt.Println("Error:", msg)
	os.Exit(1)
}

func run(kubeconfig, namespace, name string) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	new := true
	_, err = GetSecret(*clientset, namespace, name)
	if err != nil {
		if !strings.Contains(err.Error(), ("not found")) {
			log.Fatal(err)
		}
	} else {
		new = false
	}

	sess, err := session.NewSession()
	if err != nil {
		log.Fatal(err)
	}

	cv, err := sess.Config.Credentials.Get()
	if err != nil {
		log.Fatal(err)
	}

	secretData := make(map[string][]byte)
	secretData["AWS_ACCESS_KEY_ID"] = []byte(cv.AccessKeyID)
	secretData["AWS_SECRET_ACCESS_KEY"] = []byte(cv.SecretAccessKey)
	secretData["AWS_SESSION_TOKEN"] = []byte(cv.SessionToken)
	secretData["AWS_SECURITY_TOKEN"] = []byte(cv.SessionToken)

	if new {
		_, err = CreateSecret(*clientset, namespace, name, secretData)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("secret %s created successfully\n", name)
	} else {
		_, err = UpdateSecret(*clientset, namespace, name, secretData)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("secret %s updated successfully\n", name)
	}
}

func GetSecret(clientset kubernetes.Clientset, ns, name string) ([]byte, error) {
	client := clientset.CoreV1().Secrets(ns)
	s, err := client.Get(name, metav1.GetOptions{})
	if err != nil {
		return []byte{}, err
	}

	j, err := json.Marshal(s)
	if err != nil {
		return []byte{}, err
	}
	return j, nil
}

func UpdateSecret(clientset kubernetes.Clientset, ns, name string, data map[string][]byte) ([]byte, error) {
	client := clientset.CoreV1().Secrets(ns)
	s, err := client.Get(name, metav1.GetOptions{})
	if err != nil {
		return []byte{}, err
	}

	// Update data only by just pushing a new map
	s.Data = data

	s, err = client.Update(s)
	if err != nil {
		return []byte{}, err
	}

	j, err := json.Marshal(s)
	if err != nil {
		return []byte{}, err
	}
	return j, nil
}

func CreateSecret(clientset kubernetes.Clientset, ns, name string, data map[string][]byte) ([]byte, error) {
	client := clientset.CoreV1().Secrets(ns)
	secretType := corev1.SecretType("opaque")
	secretObject := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: data,
		Type: secretType,
	}

	s, err := client.Create(secretObject)
	if err != nil {
		return []byte{}, err
	}

	j, err := json.Marshal(s)
	if err != nil {
		return []byte{}, err
	}
	return j, nil
}

func main() {
	Execute((version))
}
