package terraform

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/elliotchance/sshtunnel"
	getter "github.com/hashicorp/go-getter"
	tfconfig "github.com/hashicorp/terraform/config"
	goSocks5 "github.com/isaaguilar/socks5-proxy"
	tfv1alpha1 "github.com/isaaguilar/terraform-operator/pkg/apis/tf/v1alpha1"
	giturl "github.com/whilp/git-urls"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing"
	gitauth "gopkg.in/src-d/go-git.v4/plumbing/transport"
	gitclient "gopkg.in/src-d/go-git.v4/plumbing/transport/client"
	githttp "gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	gitssh "gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
	batchv1 "k8s.io/api/batch/v1"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type k8sClient struct {
	clientset kubernetes.Interface
}

type ParsedAddress struct {
	sourcedir string
	subdirs   []string
	hash      string
	protocol  string
	uri       string
	host      string
	port      string
	user      string
	repo      string
}

type DownloadOptions struct {
	Address   string
	Directory string
	Extras    []string
	Auth      tfv1alpha1.AuthOpts
	SSHProxy  tfv1alpha1.ProxyOpts
	ParsedAddress
}

func (d *DownloadOptions) getProxyOpts(proxyOptions tfv1alpha1.ProxyOpts) {
	d.SSHProxy = proxyOptions
}

func (d *DownloadOptions) getAuthOpts(authOptions tfv1alpha1.AuthOpts) {
	d.Auth = authOptions
}

type RunOptions struct {
	mainModule       string
	moduleConfigMaps []string
	namespace        string
	name             string
	tfvarsConfigMap  string
	envVars          map[string]string
}

func newRunOptions(namespace, name string) RunOptions {
	return RunOptions{
		namespace: namespace,
		name:      name,
		envVars:   make(map[string]string),
	}
}

func (r *RunOptions) updateDownloadedModules(module string) {
	r.moduleConfigMaps = append(r.moduleConfigMaps, module)
}

func (r *RunOptions) updateEnvVars(k, v string) {
	r.envVars[k] = v
}

var log = logf.Log.WithName("controller_terraform")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Terraform Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileTerraform{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("terraform-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Terraform
	err = c.Watch(&source.Kind{Type: &tfv1alpha1.Terraform{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for terraform job completions
	err = c.Watch(&source.Kind{Type: &batchv1.Job{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &tfv1alpha1.Terraform{},
	})
	if err != nil {
		return err
	}

	// // Watch for changes to secondary resource Pods and requeue the owner Terraform
	// err = c.Watch(&source.Kind{Type: &apiv1.Pod{}}, &handler.EnqueueRequestForOwner{
	// 	IsController: true,
	// 	OwnerType:    &tfv1alpha1.Terraform{},
	// })
	// if err != nil {
	// 	return err
	// }

	return nil
}

// blank assignment to verify that ReconcileTerraform implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileTerraform{}

// ReconcileTerraform reconciles a Terraform object
type ReconcileTerraform struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Terraform object and makes changes based on the state read
// and what is in the Terraform.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileTerraform) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Terraform")

	// I set up a client here based on the only way I knew how to set up a
	// client before... Instead I'm going to try and  recycle the
	// runtime-controller client
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	job := k8sClient{
		clientset: clientset,
	}

	// Fetch the Terraform instance
	instance := &tfv1alpha1.Terraform{}
	err = r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("Terraform resource not found. Ignoring since object must be deleted")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Error(err, "Failed to get Terraform")
		return reconcile.Result{}, err
	}

	// Check if the deployment already exists, if not create a new one
	found := &batchv1.Job{}
	err = r.client.Get(context.TODO(), request.NamespacedName, found)
	if err != nil && errors.IsNotFound(err) {
		// =-=-=-=-=- MEMCACHED EXAMPLE STEP: Define a new deployment =-=-=-=-
		// dep := r.deploymentForMemcached(memcached)

		runOpts := newRunOptions(instance.Namespace, instance.Name)
		// runOpts.namespace = instance.Namespace

		// Stack Download
		reqLogger.Info("Reading spec.stack config")
		if (tfv1alpha1.TerraformStack{}) == *instance.Spec.Stack {
			return reconcile.Result{}, fmt.Errorf("No stack source defined")
		} // else if (tfv1alpha1.SrcOpts{}) == *instance.Spec.Stack.Source {
		// 	return reconcile.Result{}, fmt.Errorf("No stack source defined")
		// }
		stackDownloadOptions, err := newDownloadOptionsFromSpec(instance, instance.Spec.Stack.Source)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("Error in newDownloadOptionsFromSpec: %v", err)
		}
		reqLogger.Info("Downloading Stack")
		err = stackDownloadOptions.download(job, instance.Namespace)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("Error in download: %v", err)
		}
		reqLogger.Info(fmt.Sprintf("Stack was downloaded and updated DownloadOptions: %+v", stackDownloadOptions))

		// Download module sources. This is usually handled by terraform but
		// go-getter (the hashicorp tool to fetch sources) would need some kind of
		// vpn or transparent proxy to get resources behind some firewalls. Even
		// though it can be handled by the terraform worker pod, this option will
		// download all the sources and create configmaps that get injected into
		// the terraform worker pod so go-getter does not have to make any fetches.
		if ListContainsStr(stackDownloadOptions.Extras, "get-module-sources") {
			err = stackDownloadOptions.downloadSource(job, instance.Namespace, instance, []string{}, &runOpts)
			if err != nil {
				return reconcile.Result{}, fmt.Errorf("Error in downloadSource: %v", err)
			}
		} else {
			runOpts.updateDownloadedModules(stackDownloadOptions.hash)
		}
		runOpts.mainModule = stackDownloadOptions.hash
		reqLogger.Info(fmt.Sprintf("All moduleConfigMaps: %v", runOpts.moduleConfigMaps))

		// Download the tfvar configs
		reqLogger.Info("Reading spec.config ")
		// TODO Validate spec.config exists
		// TODO validate spec.config.sources exists && len > 0
		tfvars := ""
		for _, s := range instance.Spec.Config.Sources {
			// Loop thru all the sources in spec.config
			configDownloadOptions, err := newDownloadOptionsFromSpec(instance, s)
			if err != nil {
				return reconcile.Result{}, fmt.Errorf("Error in newDownloadOptionsFromSpec: %v", err)
			}
			err = configDownloadOptions.download(job, instance.Namespace)
			if err != nil {
				return reconcile.Result{}, fmt.Errorf("Error in download: %v", err)
			}
			reqLogger.Info(fmt.Sprintf("Config was downloaded and updated DownloadOptions: %+v", configDownloadOptions))

			tfvarSource, err := configDownloadOptions.tfvarFiles()
			if err != nil {
				return reconcile.Result{}, fmt.Errorf("Error in reading tfvarFiles: %v", err)
			}
			tfvars += tfvarSource
		}
		data := make(map[string]string)
		data["tfvars"] = tfvars
		tfvarsConfigMap := instance.Name + "-tfvars"
		// Save the vars as a configmap so a user can query kube without having
		// to go find the configs from the download sources.
		// Q. Shoud this just be as ephemeral as the modules?
		err = job.createConfigMap(tfvarsConfigMap, instance.Namespace, make(map[string][]byte), data)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("Could not create configmap %v", err)
		}
		runOpts.tfvarsConfigMap = tfvarsConfigMap

		// TODO Validate spec.config.env
		for _, env := range instance.Spec.Config.Env {
			runOpts.updateEnvVars(env.Name, env.Value)
		}

		reqLogger.Info(fmt.Sprintf("Ready to run terraform with run options: %+v", runOpts))

		// dep := runOpts.run()

		// controllerutil.SetControllerReference(instance, dep, r.scheme)
		// reqLogger.Info("Creating a new Job", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)

		// err = r.client.Create(context.TODO(), dep)
		// if err != nil {
		// 	reqLogger.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		// 	return reconcile.Result{}, err
		// }
		// // Job created successfully - return and requeue

		// TODO make sure that another run does not
		// interfere with a currently running execution of the same run
		err = runOpts.run()
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("Terraform Run did not finish successfully: %v", err)
		}

		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get Job")
		return reconcile.Result{}, err
	}

	if found.Status.Active != 0 {
		// The terraform is still being executed, wait until 0 active
		return reconcile.Result{Requeue: true}, nil
	}

	if found.Status.Succeeded > 0 {
		// The terraform is still being executed, wait until 0 active
		cm, err := job.readConfigMap(instance.Name+"-status", instance.Namespace)
		if err != nil {
			return reconcile.Result{}, err
		}
		reqLogger.Info(fmt.Sprintf("Setting status of terraform plan as %v", cm.Data))
		return reconcile.Result{}, nil
	}

	// TODO How is tfstate handled? Do I create volume for tfstate? Store it in s3? Can I use a backend config? Do I need backends?
	// TODO run terraform when CR changes
	// TODO auto start reconciliation
	// TODO figure out what triggers apply/destroy

	return reconcile.Result{}, nil
}

func (r RunOptions) run() error {
	reqLogger := log.WithValues("function", "run")
	reqLogger.Info(fmt.Sprintf("Running job with this setup: %+v", r))

	// // Get data["tfvars"] from tfvars configmap
	// cm, err := job.readConfigMap(instance.Name+"-status", instance.Namespace)
	// if err != nil {
	// 	return reconcile.Result{}, err
	// }
	reqLogger.Info("use a client to get a configmap")

	pvDir := "/tmp/modules"

	// This doesn't actually do anything, I'm just putting some placeholders in
	// for when I write the actual command. Making sure the print statements
	// look correct
	reqLogger.Info("cd " + pvDir + "/" + r.mainModule)
	reqLogger.Info("terraform init .")
	// TODO continue the terraform planning WIP

	return job
}

func newDownloadOptionsFromSpec(instance *tfv1alpha1.Terraform, source *tfv1alpha1.SrcOpts) (DownloadOptions, error) {
	d := DownloadOptions{}
	var sshProxyOptions tfv1alpha1.ProxyOpts
	var tfAuthOptions tfv1alpha1.AuthOpts

	// TODO allow configmaps as a source. This has to be parsed differently
	// before being passed to terraform's parsing mechanism

	temp, err := ioutil.TempDir("", "repo")
	if err != nil {
		return d, fmt.Errorf("Unable to make directory: %v", err)
	}
	defer os.RemoveAll(temp) // clean up

	d = DownloadOptions{
		Address:   source.Address,
		Directory: temp,
		Extras:    source.Extras,
	}

	if source.SSHProxy != nil {
		if source.SSHProxy.Host != "" {
			sshProxyOptions = tfv1alpha1.ProxyOpts{
				Host: source.SSHProxy.Host,
				User: source.SSHProxy.User,
				Auth: source.SSHProxy.Auth,
			}
		}
	} else if instance.Spec.SSHProxy != nil {
		sshProxyOptions = *instance.Spec.SSHProxy
	}

	d.getProxyOpts(sshProxyOptions)

	if source.Auth != nil {
		tfAuthOptions = *source.Auth
	} else if instance.Spec.Auth != nil {
		tfAuthOptions = *instance.Spec.Auth
	}
	d.getAuthOpts(tfAuthOptions)

	return d, nil
}

func getHostKey(host string) (ssh.PublicKey, error) {
	file, err := os.Open(filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts"))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var hostKey ssh.PublicKey
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), " ")
		if len(fields) != 3 {
			continue
		}
		if strings.Contains(fields[0], host) {
			var err error
			hostKey, _, _, _, err = ssh.ParseAuthorizedKey(scanner.Bytes())
			if err != nil {
				return nil, fmt.Errorf("error parsing %q: %v", fields[2], err)
			}
			break
		}
	}

	if hostKey == nil {
		return nil, fmt.Errorf("no hostkey for %s", host)
	}
	return hostKey, nil
}

func (d DownloadOptions) tfvarFiles() (string, error) {
	// dump contents of tfvar files into a var
	tfvars := ""

	// TODO Should path definitions walk the path?
	if ListContainsStr(d.Extras, "subdirs-as-files") {
		for _, filename := range d.subdirs {
			file := filepath.Join(d.Directory, filename)
			content, err := ioutil.ReadFile(file)
			if err != nil {
				return "", fmt.Errorf("error reading file: %v", err)
			}
			tfvars += string(content) + "\n"
		}
	} else if len(d.subdirs) > 0 {
		for _, s := range d.subdirs {
			subdir := filepath.Join(d.Directory, s)
			lsdir, err := ioutil.ReadDir(subdir)
			if err != nil {
				return "", fmt.Errorf("error listing dir: %v", err)
			}

			for _, f := range lsdir {
				if strings.Contains(f.Name(), ".tfvars") {
					file := filepath.Join(subdir, f.Name())

					content, err := ioutil.ReadFile(file)
					if err != nil {
						return "", fmt.Errorf("error reading file: %v", err)
					}

					tfvars += string(content) + "\n"
				}
			}
		}
	} else {
		lsdir, err := ioutil.ReadDir(d.Directory)
		if err != nil {
			return "", fmt.Errorf("error listing dir: %v", err)
		}

		for _, f := range lsdir {
			if strings.Contains(f.Name(), ".tfvars") {
				file := filepath.Join(d.Directory, f.Name())

				content, err := ioutil.ReadFile(file)
				if err != nil {
					return "", fmt.Errorf("error reading file: %v", err)
				}

				tfvars += string(content) + "\n"
			}
		}
	}
	// TODO validate tfvars
	return tfvars, nil
}

// downloadFromSource will downlaod the files locally. It will also download
// tf modules locally if the user opts to. TF module downloading
// is probably going to be used in the event that go-getter cannot fetch the
// modules, perhaps becuase of a firewall. Check for proxy settings to send
// to the download command.
func downloadFromSource(src, moduleDir string) error {

	// Check for global proxy

	ds := getter.Detectors
	output, err := getter.Detect(src, moduleDir, ds)
	if err != nil {
		return fmt.Errorf("Could not Detect source: %v", err)
	}

	if strings.HasPrefix(output, "git::") {
		// send to gitSource
		return fmt.Errorf("There isn't an error, reading output as %v", output)
	} else if strings.HasPrefix(output, "https://") {
		return fmt.Errorf("downloadFromSource does not yet support http(s)")
	} else if strings.HasPrefix(output, "file://") {
		return fmt.Errorf("downloadFromSource does not yet support file")
	} else if strings.HasPrefix(output, "s3::") {
		return fmt.Errorf("downloadFromSource does not yet support s3")
	}

	// TODO If the total size of the stacks configmap is too large, it will have
	// to uploaded else where.

	return nil
}

func configureGitSSHString(user, host, port, uri string) string {
	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri
	}
	return fmt.Sprintf("ssh://%s@%s:%s%s", user, host, port, uri)
}

func tarBinaryData(fullpath, filename string) (map[string][]byte, error) {
	binaryData := make(map[string][]byte)
	// Archive the file and send to configmap
	// First remove the .git file if exists in Path
	gitFile := filepath.Join(fullpath, ".git")
	_, err := os.Stat(gitFile)
	if err == nil {
		if err = os.RemoveAll(gitFile); err != nil {
			return binaryData, fmt.Errorf("Could not find or remove .git: %v", err)
		}
	}

	tardir, err := ioutil.TempDir("", "tarball")
	if err != nil {
		return binaryData, fmt.Errorf("unable making tardir: %v", err)
	}
	defer os.RemoveAll(tardir) // clean up

	tarTarget := filepath.Join(tardir, "tarball")
	tarSource := filepath.Join(tardir, filename)

	err = os.Mkdir(tarTarget, 0755)
	if err != nil {
		return binaryData, fmt.Errorf("Could not create tarTarget: %v", err)
	}
	err = os.Mkdir(tarSource, 0755)
	if err != nil {
		return binaryData, fmt.Errorf("Could not create tarTarget: %v", err)
	}

	// expect result of untar to be same as filename. Copy src to a
	// "filename" dir instead of it's current dir
	// targetSrc := filepath.Join(target, fmt.Sprintf("%s", filename))
	err = CopyDirectory(fullpath, tarSource)
	if err != nil {
		return binaryData, err
	}

	err = tarit("repo", tarSource, tarTarget)
	if err != nil {
		return binaryData, fmt.Errorf("error archiving '%s': %v", tarSource, err)
	}
	// files := make(map[string][]byte)
	tarballs, err := ioutil.ReadDir(tarTarget)
	if err != nil {
		return binaryData, fmt.Errorf("error listing tardir: %v", err)
	}
	for _, f := range tarballs {
		content, err := ioutil.ReadFile(filepath.Join(tarTarget, f.Name()))
		if err != nil {
			return binaryData, fmt.Errorf("error reading tarball: %v", err)
		}

		binaryData[f.Name()] = content
	}

	return binaryData, nil
}

func (c *k8sClient) readConfigMap(name, namespace string) (*apiv1.ConfigMap, error) {

	configMap, err := c.clientset.CoreV1().ConfigMaps(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return &apiv1.ConfigMap{}, fmt.Errorf("error reading configmap: %v", err)
	}

	return configMap, nil
}

func (c *k8sClient) createConfigMap(name, namespace string, binaryData map[string][]byte, data map[string]string) error {

	configMapObject := &apiv1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data:       data,
		BinaryData: binaryData,
	}

	_, err := c.clientset.CoreV1().ConfigMaps(namespace).Create(configMapObject)
	if err != nil {
		_, err = c.clientset.CoreV1().ConfigMaps(namespace).Update(configMapObject)
		if err != nil {
			return fmt.Errorf("error creating configmap: %v", err)
		}
	}

	return nil
}

func (c *k8sClient) loadPassword(key, name, namespace string) (string, error) {
	secret, err := c.clientset.CoreV1().Secrets(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("Could not get secret: %v", err)
	}

	var password []byte
	for k, value := range secret.Data {
		if k == key {
			password = value
		}
	}

	if len(password) == 0 {
		return "", fmt.Errorf("unable to locate '%s' in secret: %v", key, err)
	}

	return string(password), nil
}

func (c *k8sClient) loadPrivateKey(key, name, namespace string) (*os.File, error) {
	secret, err := c.clientset.CoreV1().Secrets(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("Could not get id_rsa secret: %v", err)
	}

	var privateKey []byte
	for k, value := range secret.Data {
		if k == key {
			privateKey = value
		}
	}

	if len(privateKey) == 0 {
		return nil, fmt.Errorf("unable to locate '%s' in secret: %v", key, err)
	}

	content := []byte(privateKey)
	tmpfile, err := ioutil.TempFile("", "id_rsa")
	if err != nil {
		return nil, fmt.Errorf("error creating tmpfile: %v", err)
	}

	if _, err := tmpfile.Write(content); err != nil {
		return nil, fmt.Errorf("unable to write tempfile: %v", err)
	}

	var mode os.FileMode
	mode = 0600
	os.Chmod(tmpfile.Name(), mode)

	return tmpfile, nil
}

func unique(s []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range s {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

func tarit(filename, source, target string) error {
	reqLogger := log.WithValues("function", "tarit", "filename", filename)

	target = filepath.Join(target, fmt.Sprintf("%s.tar", filename))
	tarfile, err := os.Create(target)
	if err != nil {
		return err
	}
	defer tarfile.Close()

	tarball := tar.NewWriter(tarfile)
	defer tarball.Close()

	info, err := os.Stat(source)
	if err != nil {
		return nil
	}
	reqLogger.Info(fmt.Sprintf(""))

	var baseDir string
	if info.IsDir() {
		baseDir = filepath.Base(source)
	}

	return filepath.Walk(source,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			header, err := tar.FileInfoHeader(info, info.Name())
			if err != nil {
				return err
			}

			if baseDir != "" {
				header.Name = filepath.Join(baseDir, strings.TrimPrefix(path, source))
			}

			if err := tarball.WriteHeader(header); err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(tarball, file)
			return err
		})
}

func untar(tarball, target string) error {
	reader, err := os.Open(tarball)
	if err != nil {
		return err
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		path := filepath.Join(target, header.Name)
		info := header.FileInfo()
		if info.IsDir() {
			if err = os.MkdirAll(path, info.Mode()); err != nil {
				return err
			}
			continue
		}

		file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(file, tarReader)
		if err != nil {
			return err
		}
	}
	return nil
}

func ListContainsStr(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func (d DownloadOptions) downloadSource(job k8sClient, namespace string, instance *tfv1alpha1.Terraform, downloadedSources []string, r *RunOptions) error {
	// Make sure we want to download the module sources
	// if !instance.Spec.Stack.Source.getModuleSources {
	// 	return fmt.Errorf("Skip fetching module sources")
	// }
	reqLogger := log.WithValues("Namespace", namespace, "Function", "downloadSource")
	fullpath := filepath.Join(d.Directory, d.subdirs[0])

	tfConfigs, err := tfconfig.LoadDir(fullpath)
	if err != nil {
		return fmt.Errorf("Unable to LoadDir %s because %v", fullpath, err)
	}

	replacements := make(map[string]string)
	for _, m := range tfConfigs.Modules {
		// find the terraform method used to parse Sources
		if strings.Contains(m.Source, "http") || strings.Contains(m.Source, "ssh") || strings.Contains(m.Source, "git@") {
			if replacements[m.Source] != "" {
				continue
			}

			var sshProxyOptions tfv1alpha1.ProxyOpts
			var tfAuthOptions tfv1alpha1.AuthOpts
			directory, err := ioutil.TempDir("", "repo")
			if err != nil {
				return fmt.Errorf("Unable to make directory: %v", err)
			}
			defer os.RemoveAll(directory) // clean up

			moduleDownloadOptions := DownloadOptions{
				Address:   m.Source,
				Directory: directory,
			}

			if instance.Spec.SSHProxy != nil {
				sshProxyOptions = *instance.Spec.SSHProxy
			}
			moduleDownloadOptions.getProxyOpts(sshProxyOptions)

			if instance.Spec.Auth != nil {
				tfAuthOptions = *instance.Spec.Auth
			}
			moduleDownloadOptions.getAuthOpts(tfAuthOptions)

			err = moduleDownloadOptions.download(job, instance.Namespace)
			if err != nil {
				return fmt.Errorf("Error in download: %v", err)
			}
			reqLogger.Info(fmt.Sprintf("Module was downloaded and updated DownloadOptions: %+t", moduleDownloadOptions))

			// Iterate thru all the files and download modules
			// TODO make sure we don't end up in an endless loop if we see the same module being downloaded

			if ListContainsStr(downloadedSources, m.Source) {
				replacements[m.Source] = "/" + moduleDownloadOptions.hash
				continue
			} else {
				downloadedSources = append(downloadedSources, m.Source)

				err = moduleDownloadOptions.downloadSource(job, namespace, instance, downloadedSources, r)
				if err != nil {
					return err
				}
			}

			// Will this update the module source file?
			// Update source using basic search/replace functionality in dir
			replacements[m.Source] = "/" + moduleDownloadOptions.hash

			// if (tfv1alpha1.ProxyOpts{}) != d.SSHProxy {
			// 	reqLogger.Info(fmt.Sprintf("Downloading %+v to .__modules__", m.Source))
			// }

		}
		// reqLogger.Info(fmt.Sprintf("Downloading %+v", m))
	}

	if len(replacements) > 0 {
		files, err := ioutil.ReadDir(fullpath)
		if err != nil {
			return fmt.Errorf("Error with ReadDir: %v", err)
		}

		for _, f := range files {
			if !f.IsDir() {
				filename := filepath.Join(fullpath, f.Name())
				input, err := ioutil.ReadFile(filename)
				if err != nil {
					return fmt.Errorf("Error with ReadFile: %v", err)
				}

				lines := strings.Split(string(input), "\n")

				for i, line := range lines {
					if strings.Contains(line, "source") {
						for k, v := range replacements {
							if strings.Contains(line, k) {
								lines[i] = string(bytes.Replace([]byte(lines[i]), []byte(k), []byte(v), -1))
							}
						}

					}
				}
				output := strings.Join(lines, "\n")
				err = ioutil.WriteFile(filename, []byte(output), 0644)
				if err != nil {
					return fmt.Errorf("Error with WriteFile: %v", err)
				}
			}

		}
	}

	// Write all to a configmap
	// binaryData, err := tarBinaryData(fullpath, d.hash)
	// if err != nil {
	// 	return fmt.Errorf("Error creating binary data: %v", err)
	// }
	//
	// err = job.createConfigMap(d.hash, namespace, binaryData, make(map[string]string))
	// if err != nil {
	// 	return fmt.Errorf("Error creating binary data: %v", err)
	// }

	pvDir := "/tmp/modules" // TODO use a better file location for persistence
	err = CopyDirectory(fullpath, filepath.Join(pvDir+"/"+d.hash))
	if err != nil {
		return fmt.Errorf("Error copying module dir to pv: %v", err)
	}

	r.updateDownloadedModules(d.hash)
	reqLogger.Info(fmt.Sprintf("Created configmap for %s", d.hash))
	return nil
}

func (d *DownloadOptions) download(job k8sClient, namespace string) error {
	reqLogger := log.WithValues("Download", d.Address, "Namespace", namespace, "Function", "download")
	reqLogger.Info("Starting download function")
	err := d.getParsedAddress()
	if err != nil {
		return fmt.Errorf("Error parsing address: %v", err)
	}
	repo := d.repo
	uri := d.uri

	if (tfv1alpha1.ProxyOpts{}) != d.SSHProxy {
		reqLogger.Info("Setting up a proxy")
		proxyAuthMethod, err := d.getProxyAuthMethod(job, namespace)
		if err != nil {
			return fmt.Errorf("Error getting proxyAuthMethod: %v", err)
		}

		if strings.Contains(d.protocol, "http") {
			proxyServer := fmt.Sprintf("%s:22", d.SSHProxy.Host)
			if strings.Contains(d.host, ":") {
				proxyServer = d.SSHProxy.Host
			}

			hostKey := goSocks5.NewHostKey()
			duration := time.Duration(60)
			socks5Proxy := goSocks5.NewSocks5Proxy(hostKey, nil, duration)

			err := socks5Proxy.Start(d.SSHProxy.User, proxyServer, proxyAuthMethod)
			if err != nil {
				return fmt.Errorf("unable to start socks5: %v", err)
			}
			time.Sleep(100 * time.Millisecond)

			socks5Addr, err := socks5Proxy.Addr()
			if err != nil {
				return fmt.Errorf("unable to get socks5Addr: %v", err)
			}

			dialer, err := proxy.SOCKS5("tcp", socks5Addr, nil, proxy.Direct)
			if err != nil {
				return fmt.Errorf("unable to get dialer: %v", err)
			}

			httpTransport := &http.Transport{Dial: dialer.Dial}
			// set our socks5 as the dialer
			// httpTransport.Dial = dialer.Dial
			httpClient := &http.Client{Transport: httpTransport}

			gitclient.InstallProtocol("http", githttp.NewClient(httpClient))
			gitclient.InstallProtocol("https", githttp.NewClient(httpClient))
		} else if d.protocol == "ssh" {
			// TODO figure out how to use SOCKS5 with go-git
			proxyServerWithUser := fmt.Sprintf("%s@%s", d.SSHProxy.User, d.SSHProxy.Host)
			destination := ""
			if strings.Contains(d.host, ":") {
				destination = d.host
			} else {
				destination = fmt.Sprintf("%s:%s", d.host, d.port)
			}

			// reqLogger.Info(fmt.Sprintf("Setting up proxy: %s@%s", user, host))

			// Setup the tunnel, but do not yet start it yet.
			tunnel := sshtunnel.NewSSHTunnel(
				// User and host of tunnel server, it will default to port 22
				// if not specified.
				proxyServerWithUser,

				// Pick ONE of the following authentication methods:
				// sshtunnel.PrivateKeyFile(filepath.Join(os.Getenv("HOME"), ".ssh", "id_rsa")), // 1. private key
				proxyAuthMethod,

				// The destination host and port of the actual server.
				destination,
			)

			// NewSSHTunnel will bind to a random port so that you can have
			// multiple SSH tunnels available. The port is available through:
			//   tunnel.Local.Port

			// You can use any normal Go code to connect to the destination server
			// through localhost. You may need to use 127.0.0.1 for some libraries.

			// You can provide a logger for debugging, or remove this line to
			// make it silent.
			// tunnel.Log = log.New(os.Stdout, "", log.Ldate|log.Lmicroseconds)
			// reqLogger.Info(tunnel.Log)

			// Start the server in the background. You will need to wait a
			// small amount of time for it to bind to the localhost port
			// before you can start sending connections.
			go tunnel.Start()
			time.Sleep(100 * time.Millisecond)

			port := strconv.Itoa(tunnel.Local.Port)

			if strings.Index(uri, "/") != 0 {
				uri = "/" + uri
			}

			// // configure auth with go git options
			repo = fmt.Sprintf("ssh://%s@127.0.0.1:%s%s", d.user, port, uri)
		}
	}
	reqLogger.Info(fmt.Sprintf("Getting ready to download source %s", repo))

	gitAuthMethod, err := d.getGitAuthMethod(job, namespace)
	if err != nil {
		return fmt.Errorf("Error getting gitAuthMethod: %v", err)
	}

	gitConfigs := git.CloneOptions{
		Auth:              gitAuthMethod,
		URL:               repo,
		RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
		ReferenceName:     "refs/heads/master",
	}

	reqLogger.Info("Validating git config")
	err = gitConfigs.Validate()
	if err != nil {
		return fmt.Errorf("Git config not valid: %v", err)
	}

	reqLogger.Info("Git Clone (PlainClone)")
	r, err := git.PlainClone(d.Directory, false, &gitConfigs)
	if err != nil {
		return fmt.Errorf("Could not checkout repo: %v", err)
	}

	w, err := r.Worktree()
	if err != nil {
		return fmt.Errorf("Could not get Worktree: %v", err)
	}

	err = r.Fetch(&git.FetchOptions{
		Auth:     gitAuthMethod,
		RefSpecs: []config.RefSpec{"refs/*:refs/*", "HEAD:refs/heads/HEAD"},
	})
	if err != nil {
		return fmt.Errorf("Could not Fetch: %v", err)
	}

	if d.hash != "" {
		commit := plumbing.NewHash(d.hash)
		reqLogger.Info(fmt.Sprintf("Checking out hash: %v", commit))
		err = w.Checkout(&git.CheckoutOptions{
			Hash: commit,
		})
		if err != nil {
			return fmt.Errorf("Could not checkout ref '%s': %v", commit, err)
		}
	}

	ref, err := r.Head()
	if err != nil {
		return fmt.Errorf("Could not find ref: %v", err)
	}
	hash := ref.Hash().String()

	// Set the hash and return
	d.hash = hash
	return nil
}

func (d *DownloadOptions) getParsedAddress() error {
	sourcedir, subdirstr := getter.SourceDirSubdir(d.Address)
	// subdir can contain a list seperated by double slashes
	subdirs := strings.Split(subdirstr, "//")
	src := strings.TrimPrefix(sourcedir, "git::")
	var hash string
	if strings.Contains(sourcedir, "?") {
		for i, v := range strings.Split(sourcedir, "?") {
			if i > 0 {
				if strings.Contains(v, "&") {
					for _, w := range strings.Split(v, "&") {
						if strings.Contains(w, "ref=") {
							hash = strings.Split(w, "ref=")[1]
						}
					}

				} else if strings.Contains(v, "ref=") {
					hash = strings.Split(v, "ref=")[1]
				}
			}

		}
	}

	// strip out the url args
	repo := strings.Split(src, "?")[0]
	u, err := giturl.Parse(repo)
	if err != nil {
		return fmt.Errorf("unable to parse giturl: %v", err)
	}
	protocol := u.Scheme
	uri := strings.Split(u.RequestURI(), "?")[0]
	host := u.Host
	port := u.Port()
	if port == "" {
		if protocol == "ssh" {
			port = "22"
		} else if protocol == "https" {
			port = "443"
		}
	}

	user := u.User.Username()
	if user == "" {
		user = "git"
	}

	d.ParsedAddress = ParsedAddress{
		sourcedir: sourcedir,
		subdirs:   subdirs,
		hash:      hash,
		protocol:  protocol,
		uri:       uri,
		host:      host,
		port:      port,
		user:      user,
		repo:      repo,
	}
	return nil
}

func (d DownloadOptions) getProxyAuthMethod(job k8sClient, namespace string) (ssh.AuthMethod, error) {
	var proxyAuthMethod ssh.AuthMethod
	if d.SSHProxy.Auth.Type == "key" {
		sshKey, err := job.loadPrivateKey(d.SSHProxy.Auth.Key, d.SSHProxy.Auth.Name, namespace)
		if err != nil {
			return proxyAuthMethod, fmt.Errorf("unable to get privkey: %v", err)
		}
		defer os.Remove(sshKey.Name())
		defer sshKey.Close()
		proxyAuthMethod = sshtunnel.PrivateKeyFile(sshKey.Name())
	} else if d.SSHProxy.Auth.Type == "password" {
		password, err := job.loadPassword(d.SSHProxy.Auth.Key, d.SSHProxy.Auth.Name, namespace)
		if err != nil {
			return proxyAuthMethod, fmt.Errorf("unable to get proxy password: %v", err)
		}
		proxyAuthMethod = ssh.Password(password)
	}
	return proxyAuthMethod, nil
}

func (d *DownloadOptions) getGitAuthMethod(job k8sClient, namespace string) (gitauth.AuthMethod, error) {
	var auth gitauth.AuthMethod
	if d.Auth.Type == "key" {
		sshKey, _ := job.loadPrivateKey(d.Auth.Key, d.Auth.Name, namespace)
		defer os.Remove(sshKey.Name())
		defer sshKey.Close()

		key, err := ioutil.ReadFile(sshKey.Name())
		if err != nil {
			return auth, fmt.Errorf("unable to read private key: %v", err)
		}

		// Create the Signer for this private key.
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return auth, fmt.Errorf("unable to parse private key: %v", err)
		}

		auth = &gitssh.PublicKeys{
			User:   d.user,
			Signer: signer,
			HostKeyCallbackHelper: gitssh.HostKeyCallbackHelper{
				HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			},
		}
	} else if d.Auth.Type == "password" {
		gitPassword, err := job.loadPassword(d.Auth.Key, d.Auth.Name, namespace)
		if err != nil {
			return auth, fmt.Errorf("unable to get password: %v", err)
		}
		auth = &githttp.BasicAuth{
			Username: d.user,
			Password: gitPassword,
		}
	}
	return auth, nil
}

func CopyDirectory(scrDir, dest string) error {
	entries, err := ioutil.ReadDir(scrDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(scrDir, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		fileInfo, err := os.Stat(sourcePath)
		if err != nil {
			return err
		}

		stat, ok := fileInfo.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("failed to get raw syscall.Stat_t data for '%s'", sourcePath)
		}

		switch fileInfo.Mode() & os.ModeType {
		case os.ModeDir:
			if err := CreateIfNotExists(destPath, 0755); err != nil {
				return err
			}
			if err := CopyDirectory(sourcePath, destPath); err != nil {
				return err
			}
		case os.ModeSymlink:
			if err := CopySymLink(sourcePath, destPath); err != nil {
				return err
			}
		default:
			if err := Copy(sourcePath, destPath); err != nil {
				return err
			}
		}

		if err := os.Lchown(destPath, int(stat.Uid), int(stat.Gid)); err != nil {
			return err
		}

		isSymlink := entry.Mode()&os.ModeSymlink != 0
		if !isSymlink {
			if err := os.Chmod(destPath, entry.Mode()); err != nil {
				return err
			}
		}
	}
	return nil
}

func Copy(srcFile, dstFile string) error {
	out, err := os.Create(dstFile)
	defer out.Close()
	if err != nil {
		return err
	}

	in, err := os.Open(srcFile)
	defer in.Close()
	if err != nil {
		return err
	}

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}

	return nil
}

func Exists(filePath string) bool {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return false
	}

	return true
}

func CreateIfNotExists(dir string, perm os.FileMode) error {
	if Exists(dir) {
		return nil
	}

	if err := os.MkdirAll(dir, perm); err != nil {
		return fmt.Errorf("failed to create directory: '%s', error: '%s'", dir, err.Error())
	}

	return nil
}

func CopySymLink(source, dest string) error {
	link, err := os.Readlink(source)
	if err != nil {
		return err
	}
	return os.Symlink(link, dest)
}
