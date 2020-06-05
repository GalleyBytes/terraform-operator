package gitclient

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"time"

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
	repo *git.Repository
	ref  *plumbing.Reference
}

func (g *GitRepo) HashString() (string, error) {
	if g.ref == nil {
		return "", fmt.Errorf("The GitRepo.ref has not been set")
	}
	return g.ref.Hash().String(), nil

}

func (g *GitRepo) checkout(commit string) error {
	w, err := g.repo.Worktree()
	if err != nil {
		return fmt.Errorf("Could not get Worktree: %v", err)
	}

	// Check if this is a hash
	n, _ := strconv.ParseUint(commit, 16, 64)
	if len(commit) == 40 && n > 0 {
		fmt.Printf("checking out hash: %v\n", commit)
		// Try checking out a hash commit
		err = w.Checkout(&git.CheckoutOptions{
			Hash: plumbing.NewHash(commit),
		})
		if err != nil {
			return fmt.Errorf("Error checking out hash: %v", err)
		}
	} else {
		fmt.Printf("checking out branch: %v\n", "refs/heads/"+commit)
		// Try checking out a branch
		err = w.Checkout(&git.CheckoutOptions{
			Branch: plumbing.ReferenceName("refs/heads/" + commit),
		})
		if err != nil {
			return fmt.Errorf("Error checking out branch: %v", err)
		}
	}
	ref, err := g.repo.Head()
	if err != nil {
		return fmt.Errorf("Error reading head: %v", err)
	}
	g.ref = ref
	return nil
}

func (g *GitRepo) downloadGitRepo(url, repoDir string) error {

	gitConfigs := git.CloneOptions{
		Auth:              g.auth,
		URL:               url,
		RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
		ReferenceName:     "refs/heads/master",
		Progress:          os.Stdout,
	}

	err := gitConfigs.Validate()
	if err != nil {
		return fmt.Errorf("Git config not valid: %v", err)
	}

	r, err := git.PlainClone(repoDir, false, &gitConfigs)
	if err != nil {
		return fmt.Errorf("Could not checkout repo: %v", err)
	}

	// Checkout the git-refs used for checkouts
	err = r.Fetch(&git.FetchOptions{
		Auth:     g.auth,
		RefSpecs: []config.RefSpec{"refs/*:refs/*", "HEAD:refs/heads/HEAD"},
	})
	if err != nil {
		return fmt.Errorf("Could not Fetch: %v", err)
	}
	g.repo = r

	ref, err := r.Head()
	if err != nil {
		return fmt.Errorf("Error reading head: %v", err)
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
		return auth, fmt.Errorf("unable to get gitauth Method\n")
	}

	return auth, nil
}

func GitHTTPDownload(url, repoDir, user, password, ref string) (GitRepo, error) {
	gitRepo := GitRepo{}
	auth := passwordAuthMethod(user, password)
	gitRepo.auth = auth

	err := gitRepo.downloadGitRepo(url, repoDir)
	if err != nil {
		return gitRepo, fmt.Errorf("Could not download repo: %v", err)
	}

	if ref != "" {
		gitRepo.checkout(ref)
	}
	return gitRepo, nil
}

func GitSSHDownload(url, repoDir, sshKeyFilename, ref string) (GitRepo, error) {
	gitRepo := GitRepo{}
	auth, err := sshAuthMethod(sshKeyFilename)
	if err != nil {
		return GitRepo{}, err
	}
	gitRepo.auth = auth

	err = gitRepo.downloadGitRepo(url, repoDir)
	if err != nil {
		return gitRepo, fmt.Errorf("Could not download repo: %v", err)
	}

	if ref != "" {
		gitRepo.checkout(ref)
	}
	return gitRepo, nil
}

func printRef(s *plumbing.Reference) error {
	fmt.Printf("reference: %+v\n", s)
	return nil
}

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
		return fmt.Errorf("Could not get Worktree: %v", err)
	}

	err = w.Checkout(&git.CheckoutOptions{
		Branch: b,
	})
	if err != nil {
		return fmt.Errorf("Error checking out branch: %v", err)
	}

	ref, err = g.repo.Head()
	if err != nil {
		return fmt.Errorf("Error reading head: %v", err)
	}
	g.ref = ref
	return nil
}

func (g *GitRepo) Commit(filenames []string, message string) error {
	w, err := g.repo.Worktree()
	if err != nil {
		return fmt.Errorf("Could not get Worktree: %v", err)
	}

	for _, f := range filenames {
		fmt.Printf("Adding file %v\n", f)
		_, err := w.Add(f)
		if err != nil {
			return fmt.Errorf("Error adding file: %v", err)
		}

	}

	commit, err := w.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "devops-automation",
			Email: "devops-automation@example.com",
			When:  time.Now(),
		},
	})

	fmt.Printf("This is the commit object: %v\n", commit)

	ref, err := g.repo.Head()
	if err != nil {
		return fmt.Errorf("Error reading head: %v", err)
	}
	g.ref = ref
	return nil
}

func (g *GitRepo) Push(target plumbing.ReferenceName) error {
	fmt.Printf("Pushing to repo\n")
	if target == "" {
		target = g.ref.Name()
	}
	err := g.repo.Push(&git.PushOptions{
		RefSpecs: []config.RefSpec{config.RefSpec(g.ref.Name() + ":" + target)},
		Auth:     g.auth,
	})
	if err != nil {
		return fmt.Errorf("Error pushing branch: %v", err)
	}
	return nil
}
