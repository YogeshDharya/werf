package git_history_based_cleanup

import (
	"fmt"
	"sync"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type GitRepo interface {
	PlainOpen() (*git.Repository, error)
}

type GitRepositoryWithCache struct {
	GitRepo     *git.Repository
	CommitCache sync.Map
	mu          sync.Mutex
}

func NewGitRepositoryWithCache(gitRepo GitRepo) (*GitRepositoryWithCache, error) {
	gitRepository, err := gitRepo.PlainOpen()
	if err != nil {
		return nil, fmt.Errorf("git plain open failed: %w", err)
	}
	return &GitRepositoryWithCache{
		GitRepo: gitRepository,
	}, nil
}

// CommitObject tries to load a commit object from the cache.
// If the commit is not found in the cache, it locks the critical section where we try to load commit data from disk.
// It doesn't actually protect any data and works like a semaphore.
// Locking particular commit only is not safe because of inner go-git logic.
// We need this workaround because go-git is not thread-safe. It should be fixed in go-git v6.
// Ref: https://github.com/go-git/go-git/issues/773
func (g *GitRepositoryWithCache) CommitObject(commitHash plumbing.Hash) (*object.Commit, error) {
	if c, ok := g.getFromCache(commitHash); ok {
		return c, nil
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if c, ok := g.getFromCache(commitHash); ok {
		return c, nil
	}
	c, err := g.GitRepo.CommitObject(commitHash)
	if err != nil {
		return nil, fmt.Errorf("unable to get commit object for %s: %w", commitHash.String(), err)
	}
	g.addToCache(commitHash, c)
	return c, nil
}

func (g *GitRepositoryWithCache) ClearCache() {
	g.CommitCache.Clear()
}

func (g *GitRepositoryWithCache) addToCache(commitHash plumbing.Hash, obj *object.Commit) {
	g.CommitCache.Store(commitHash, obj)
}

func (g *GitRepositoryWithCache) getFromCache(commitHash plumbing.Hash) (*object.Commit, bool) {
	value, ok := g.CommitCache.Load(commitHash)
	if !ok {
		return nil, false
	}
	commit, ok := value.(*object.Commit)
	return commit, ok
}
