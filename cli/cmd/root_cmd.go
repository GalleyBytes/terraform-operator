package cmd

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/docker/cli/cli/streams"
	"github.com/ghodss/yaml"
	tfo "github.com/isaaguilar/terraform-operator/pkg/client/clientset/versioned"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/homedir"
	"k8s.io/kubectl/pkg/scheme"
)

type Session struct {
	config       *rest.Config
	tfoclientset tfo.Interface
	clientset    kubernetes.Interface
	namespace    string
}

func newSession() {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(err)
	}

	// Get the namespace from the user's contexts when not passed in via flag
	if namespace == "" {
		// Define the schema of the kubeconfig that is meaningful to extract
		// the current-context's namespace (if defined)
		type kubecfgContext struct {
			Namespace string `json:"namespace"`
		}
		type kubecfgContexts struct {
			Name    string         `json:"name"`
			Context kubecfgContext `json:"context"`
		}
		type kubecfg struct {
			CurrentContext string            `json:"current-context"`
			Contexts       []kubecfgContexts `json:"contexts"`
		}
		// As a kubectl plugin, it's nearly guaranteed a kubeconfig is used
		b, err := ioutil.ReadFile(kubeconfig)
		if err != nil {
			panic(err)
		}
		kubecfgCtx := kubecfg{}
		yaml.Unmarshal(b, &kubecfgCtx)
		for _, item := range kubecfgCtx.Contexts {
			if item.Name == kubecfgCtx.CurrentContext {
				namespace = item.Context.Namespace
				break
			}
		}
		if namespace == "" {
			namespace = "default"
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	tfoclientset, err := tfo.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	session.clientset = clientset
	session.tfoclientset = tfoclientset
	session.namespace = namespace
	session.config = config
}

var (
	rootCmd = &cobra.Command{
		Use:     "tfo",
		Aliases: []string{"\"kubectl tf(o)\""},
		Short:   "Terraform Operator (tfo) CLI -- Manage TFO deployments",
		// 		Long: ``,
		Args: cobra.MaximumNArgs(0),
	}

	showCmd = &cobra.Command{
		Use:   "show",
		Short: "Show a comprehensive list of tfo related resources",
		// 		Long: ``,
		Args: cobra.MaximumNArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			show("name", false)
			// fmt.Println("showing")
		},
	}

	debugCmd = &cobra.Command{
		Use:   "debug",
		Short: "Debug a tf workflow by exec into a session",
		// 		Long: ``,
		// Args: cobra.MaximumNArgs(1),
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := args[0]
			debug(name)
		},
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version of this bin",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(os.Stderr, "tfo-")
			fmt.Printf("%s\n", version)
		},
	}
)

var (
	// vars used for flags
	session    Session
	kubeconfig string
	namespace  string
)

func init() {
	cobra.OnInitialize(newSession)
	getKubeconfig()
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "namespace to add Kubernetes creds secret")
	rootCmd.AddCommand(versionCmd, showCmd, debugCmd)
	rootCmd.CompletionOptions.DisableDefaultCmd = true
}

func getKubeconfig() {
	if home := homedir.HomeDir(); home != "" {
		rootCmd.PersistentFlags().StringVarP(&kubeconfig, "kubecfg", "c", "", "(optional) absolute path to the kubeconfig file")
		if kubeconfig == "" {
			kubeconfig = os.Getenv("KUBECONFIG")
			if kubeconfig == "" {
				kubeconfig = filepath.Join(home, ".kube", "config")
			}
		}
	} else {
		rootCmd.PersistentFlags().StringVarP(&kubeconfig, "kubecfg", "c", "", "absolute path to the kubeconfig file")
		if kubeconfig == "" {
			kubeconfig = os.Getenv("KUBECONFIG")
		}
	}

}

func er(msg interface{}) {
	fmt.Println("Error:", msg)
	os.Exit(1)
}

func show(name string, showPrevious bool) {
	tfClient := session.tfoclientset.TfV1alpha1().Terraforms(session.namespace)
	podClient := session.clientset.CoreV1().Pods(session.namespace)

	tfs, err := tfClient.List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Fatal(nil)
	}

	var data [][]string
	header := []string{"Name", "Generation", "Pods"}
	if showPrevious {
		header = append(header, "PreviousPods")
	}
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader(header)
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetBorder(false)
	table.SetTablePadding("\t") // pad with tabs
	table.SetNoWhiteSpace(true)

	for _, tf := range tfs.Items {
		data_index := len(data)
		generation := fmt.Sprintf("%d", tf.Generation)
		data = append(data, []string{tf.Name, generation, "", ""})

		pods, err := podClient.List(context.TODO(), metav1.ListOptions{
			LabelSelector: "terraforms.tf.isaaguilar.com/resourceName=" + tf.Name,
		})
		if err != nil {
			continue
		}
		var currentRunnerEntryIndex int
		var previousRunnersEntryIndex int
		for _, pod := range pods.Items {
			if pod.Labels["terraforms.tf.isaaguilar.com/generation"] == generation {
				if len(data) == data_index+currentRunnerEntryIndex {
					data = append(data, []string{"", "", pod.Name, ""})
				} else {
					data[data_index+currentRunnerEntryIndex][2] = pod.Name
				}

				currentRunnerEntryIndex++
			} else if showPrevious {
				if len(data) == data_index+previousRunnersEntryIndex {
					data = append(data, []string{"", "", "", pod.Name})
				} else {
					data[data_index+previousRunnersEntryIndex][3] = pod.Name
				}

				previousRunnersEntryIndex++
			}
		}
	}
	table.AppendBulk(data)
	table.Render()

}

func debug(name string) {
	tfClient := session.tfoclientset.TfV1alpha1().Terraforms(session.namespace)
	podClient := session.clientset.CoreV1().Pods(session.namespace)

	tf, err := tfClient.Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		log.Fatal(err)
	}

	pod := generatePod(tf)
	pod, err = podClient.Create(context.TODO(), pod, metav1.CreateOptions{})
	if err != nil {
		log.Fatal(err)
	}
	defer podClient.Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})

	fmt.Printf("Connecting to %s", pod.Name)

	watcher, err := podClient.Watch(context.TODO(), metav1.ListOptions{
		FieldSelector: "metadata.name=" + pod.Name,
	})
	if err != nil {
		log.Fatal(err)
	}

	for event := range watcher.ResultChan() {
		fmt.Printf(".")
		switch event.Type {
		case watch.Modified:
			pod = event.Object.(*corev1.Pod)
			// If the Pod contains a status condition Ready == True, stop
			// watching.
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodReady &&
					cond.Status == corev1.ConditionTrue &&
					pod.Status.Phase == corev1.PodRunning {
					watcher.Stop()
				}
			}
		default:
			// fmt.Fprintln(os.Stderr, event.Type)
		}
	}
	fmt.Println()

	req := session.clientset.CoreV1().RESTClient().
		Post().
		Namespace(pod.Namespace).
		Resource("pods").
		Name(pod.Name).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: pod.Spec.Containers[0].Name,
			Command: []string{
				"/bin/bash",
				"-c",
				"cd $TFO_MAIN_MODULE && export PS1=\"$(pwd)\\$ \" && " +
					"printf \"\nTry running 'terraform init'\n\n\" && bash",
			},
			Stdin:  true,
			Stdout: true,
			Stderr: true,
			TTY:    true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(session.config, "POST", req.URL())
	if err != nil {
		panic(err)
	}

	in := streams.NewIn(os.Stdin)
	if err := in.SetRawTerminal(); err != nil {
		panic(err)
	}
	defer in.RestoreTerminal()

	_ = exec.Stream(remotecommand.StreamOptions{
		Stdin:  in,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Tty:    true,
	})

}

// Execute executes the root command.
var version string

func Execute(v string) error {
	version = v
	return rootCmd.Execute()
}
