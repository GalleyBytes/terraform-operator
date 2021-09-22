package gitclient

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/go-logr/logr"

	"github.com/isaaguilar/terraform-operator/pkg/utils"

	"golang.org/x/crypto/ssh"
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	gitauth "gopkg.in/src-d/go-git.v4/plumbing/transport"
	githttp "gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	gitssh "gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
)

type GitRepo struct {
	auth gitauth.AuthMethod
	url  string
	repo *git.Repository
	ref  *plumbing.Reference
	Log  logr.Logger
}

func NewGitRepo(user, password, sshKeyFilename string, logger logr.Logger) (GitRepo, error) {
	var auth gitauth.AuthMethod
	var err error
	if password != "" {
		auth = passwordAuthMethod(user, password)
	}
	if sshKeyFilename != "" {
		auth, err = sshAuthMethod(sshKeyFilename)
		if err != nil {
			return GitRepo{}, err
		}
	}

	return GitRepo{
		auth: auth,
		Log:  logger,
	}, nil
}

func (g *GitRepo) HashString() (string, error) {
	if g.ref == nil {
		return "", fmt.Errorf("the GitRepo.ref has not been set")
	}
	return g.ref.Hash().String(), nil

}

func (g GitRepo) BranchName() (string, error) {
	if g.ref == nil {
		return "", fmt.Errorf("the GitRepo.ref has not been set")
	}
	if !g.ref.Name().IsBranch() {
		return "", fmt.Errorf("repo HEAD is not pointing to a branch")
	}
	return g.ref.Name().String(), nil
}

func (g *GitRepo) checkout(commit string) error {
	reqLogger := g.Log.WithValues("repo", g.url)
	w, err := g.repo.Worktree()
	if err != nil {
		return fmt.Errorf("could not get Worktree: %v", err)
	}

	// Check if this is a hash
	n, _ := strconv.ParseUint(commit, 16, 64)
	if len(commit) == 40 && n > 0 {
		reqLogger.V(1).Info(fmt.Sprintf("Checking out hash: '%v'", commit))
		// Try checking out a hash commit
		err = w.Checkout(&git.CheckoutOptions{
			Hash: plumbing.NewHash(commit),
		})
		if err != nil {
			return fmt.Errorf("error checking out hash: %v", err)
		}
	} else {
		reqLogger.V(1).Info(fmt.Sprintf("Checking out branch: '%v'", "refs/heads/"+commit))
		// Try checking out a branch
		err = w.Checkout(&git.CheckoutOptions{
			Branch: plumbing.ReferenceName("refs/heads/" + commit),
		})
		if err != nil {
			return fmt.Errorf("error checking out branch: %v", err)
		}
	}
	ref, err := g.repo.Head()
	if err != nil {
		return fmt.Errorf("error reading head: %v", err)
	}
	g.ref = ref
	return nil
}

func (g *GitRepo) downloadGitRepo(c chan error, wg *sync.WaitGroup, url, repoDir string) {
	reqLogger := g.Log.WithValues("repo", g.url)
	// Setup a logger that writes entries from an io.Writer
	progressLogger := log2logr{
		entry: g.Log.WithValues("repo", g.url),
	}

	defer wg.Done()
	defer close(c)
	gitConfigs := git.CloneOptions{
		URL:               url,
		RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
		ReferenceName:     "refs/heads/master",
		Progress:          progressLogger,
	}

	if g.auth != nil {
		gitConfigs.Auth = g.auth
	}

	err := gitConfigs.Validate()
	if err != nil {
		c <- fmt.Errorf("git config not valid: %v", err)
		return
	}

	reqLogger.V(1).Info("Cloning repo")
	r, err := git.PlainClone(repoDir, false, &gitConfigs)
	if err != nil {
		c <- fmt.Errorf("could not checkout repo: %v", err)
		return
	}
	g.repo = r

}

func (g *GitRepo) fetchRefs() error {
	reqLogger := g.Log.WithValues("repo", g.url)
	// Setup a logger that writes entries from an io.Writer
	progressLogger := log2logr{
		entry: g.Log.WithValues("repo", g.url),
	}
	// Checkout the git-refs used for checkouts
	reqLogger.V(1).Info("Fetching refs")
	err := g.repo.Fetch(&git.FetchOptions{
		Auth:     g.auth,
		RefSpecs: []config.RefSpec{"refs/*:refs/*", "HEAD:refs/heads/HEAD"},
		Progress: progressLogger,
	})
	if err != nil {
		return fmt.Errorf("could not Fetch: %v", err)
	}

	ref, err := g.repo.Head()
	if err != nil {
		return fmt.Errorf("error reading head: %v", err)
	}
	g.ref = ref
	return nil
}

func passwordAuthMethod(user, password string) gitauth.AuthMethod {
	auth := &githttp.BasicAuth{
		Username: user,
		Password: password,
	}
	return auth
}

func sshAuthMethod(sshKeyFilename string) (gitauth.AuthMethod, error) {
	var auth gitauth.AuthMethod

	sshKey, err := os.Open(sshKeyFilename)
	if err != nil {
		return auth, err
	}

	defer sshKey.Close()

	keyF, err := ioutil.ReadFile(sshKey.Name())
	if err != nil {
		return auth, err
	}

	// Create the Signer for this private key.
	signer, err := ssh.ParsePrivateKey(keyF)
	if err != nil {
		return auth, err
	}

	auth = &gitssh.PublicKeys{
		User:   "git",
		Signer: signer,
		HostKeyCallbackHelper: gitssh.HostKeyCallbackHelper{
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		},
	}

	if auth.Name() == "" {
		return auth, fmt.Errorf("unable to get gitauth Method")
	}

	return auth, nil
}

func (g *GitRepo) GitHTTPDownload(url, repoDir, user, password, ref string, timeout int64) error {
	g.url = url
	reqLogger := g.Log.WithValues("repo", url)
	reqLogger.V(1).Info("Configuring git download")
	if g.auth != nil {
		reqLogger.V(1).Info(fmt.Sprintf("Auth file set as '%s'", g.auth.Name()))
	}
	if timeout == 0 {
		timeout = int64(120)
	}
	c := make(chan error)
	var wg sync.WaitGroup
	wg.Add(1)
	go g.downloadGitRepo(c, &wg, url, repoDir)
	select {
	case err := <-c:
		if err != nil {
			return fmt.Errorf("could not download repo: %v", err)
		}
	case <-time.After(time.Duration(timeout) * time.Second):
		return fmt.Errorf("timeout occured fetching %s", url)
	}
	wg.Wait()
	reqLogger.V(1).Info("Successfully downloaded repo")

	err := g.fetchRefs()
	if err != nil {
		return err
	}
	reqLogger.V(1).Info("Successfully fetched refs")

	if ref != "" {
		g.checkout(ref)
	}
	return nil
}

func (g *GitRepo) GitSSHDownload(url, repoDir, sshKeyFilename, ref string, timeout int64) error {
	g.url = url
	reqLogger := g.Log.WithValues("repo", url)
	reqLogger.V(1).Info("Configuring git download")
	reqLogger.V(1).Info(fmt.Sprintf("Auth file set as '%s'", g.auth.Name()))
	if timeout == 0 {
		timeout = int64(120)
	}
	c := make(chan error)
	var wg sync.WaitGroup
	wg.Add(1)
	go g.downloadGitRepo(c, &wg, url, repoDir)
	select {
	case err := <-c:
		if err != nil {
			return fmt.Errorf("could not download repo: %v", err)
		}
	case <-time.After(time.Duration(timeout) * time.Second):
		return fmt.Errorf("timeout occured fetching %s", url)
	}
	wg.Wait()
	reqLogger.V(1).Info("Successfully downloaded repo")

	err := g.fetchRefs()
	if err != nil {
		return err
	}
	reqLogger.V(1).Info("Successfully fetched refs")

	if ref != "" {
		g.checkout(ref)
		reqLogger.V(1).Info(fmt.Sprintf("Checked out '%s' for '%s'", ref, url))
	}
	return nil
}

// func printRef(s *plumbing.Reference) error {
// 	fmt.Printf("reference: %+v\n", s)
// 	return nil
// }

// func GetData(filename string) (map[string]interface{}, error) {
// 	d := make(map[string]interface{})
// 	return d, nil
// }

func (g *GitRepo) CheckoutBranch(branch string) error {
	var b plumbing.ReferenceName
	if branch != "" {
		b = plumbing.ReferenceName(branch)
	} else {
		b = plumbing.ReferenceName("refs/heads/" + utils.RandomString(10))
	}

	ref := plumbing.NewHashReference(b, g.ref.Hash())
	err := g.repo.Storer.SetReference(ref)
	if err != nil {
		log.Fatal(err)
	}

	w, err := g.repo.Worktree()
	if err != nil {
		return fmt.Errorf("could not get Worktree: %v", err)
	}

	err = w.Checkout(&git.CheckoutOptions{
		Branch: b,
	})
	if err != nil {
		return fmt.Errorf("error checking out branch: %v", err)
	}

	ref, err = g.repo.Head()
	if err != nil {
		return fmt.Errorf("error reading head: %v", err)
	}
	g.ref = ref
	return nil
}

func (g *GitRepo) Commit(filenames []string, message string) error {
	reqLogger := g.Log.WithValues("repo", g.url)
	w, err := g.repo.Worktree()
	if err != nil {
		return fmt.Errorf("could not get Worktree: %v", err)
	}

	status, err := w.Status()
	if err != nil {
		return err
	}

	if status.IsClean() {
		return fmt.Errorf("no changes to commit")
	}

	filesInStatus := []string{}
	for file := range status {
		// fmt.Println("file in status", file)
		filesInStatus = append(filesInStatus, file)
	}

	isFileInStatus := false
	for _, fileToCommit := range filenames {
		// fmt.Println("file to commit", fileToCommit)
		if utils.ListContainsStr(filesInStatus, fileToCommit) {
			isFileInStatus = true
			break
		}
	}

	if !isFileInStatus {
		return fmt.Errorf("no changes to commit")
	}

	for _, f := range filenames {
		reqLogger.V(1).Info(fmt.Sprintf("Adding file %v", f))
		_, err := w.Add(f)
		if err != nil {
			return fmt.Errorf("error adding file: %v", err)
		}
	}

	commit, err := w.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "devops-automation",
			Email: "devops-automation@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		return err
	}

	reqLogger.V(1).Info(fmt.Sprintf("This is the commit object: %v", commit))

	ref, err := g.repo.Head()
	if err != nil {
		return fmt.Errorf("error reading head: %v", err)
	}
	g.ref = ref

	return nil
}

func (g *GitRepo) Push(target plumbing.ReferenceName) error {
	// Setup a logger that writes entries from an io.Writer
	progressLogger := log2logr{
		entry: g.Log.WithValues("repo", g.url),
	}
	reqLogger := g.Log.WithValues("repo", g.url)
	if target == "" {
		target = g.ref.Name()
	}
	reqLogger.V(1).Info(fmt.Sprintf("Pushing '%v' to repo", target))
	err := g.repo.Push(&git.PushOptions{
		RefSpecs: []config.RefSpec{config.RefSpec(g.ref.Name() + ":" + target)},
		Auth:     g.auth,
		Progress: progressLogger,
	})
	if err != nil {
		return fmt.Errorf("error pushing branch: %v", err)
	}
	return nil
}

// log2logr exploits the documented fact that the standard
// log pkg sends each log entry as a single io.Writer.Write call:
// https://golang.org/pkg/log/#Logger
type log2logr struct {
	entry logr.Logger
}

func (w log2logr) Write(b []byte) (int, error) {
	n := len(b)
	w.entry.V(2).Info(string(b))
	return n, nil
}
