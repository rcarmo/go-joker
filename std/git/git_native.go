package git

import (
	"unsafe"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"

	git "github.com/go-git/go-git/v5"
	gitConfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	. "github.com/rcarmo/go-joker/core"
	"github.com/rcarmo/go-joker/core/hashutil"
)

type (
	GitRepo struct {
		repo *git.Repository
		hash uint32
	}
)

var gitRepoType *coretypes.Type

func MakeGitRepo(repo *git.Repository) GitRepo {
	res := GitRepo{repo, 0}
	res.hash = hashutil.Ptr(uintptr(unsafe.Pointer(repo)))
	return res
}

func (repo GitRepo) ToString(_escape bool) string {
	return "#object[GitRepo]"
}

func (repo GitRepo) Equals(other interface{}) bool {
	if other, ok := other.(GitRepo); ok {
		return repo.repo == other.repo
	}
	return false
}

func (repo GitRepo) GetInfo() *coretypes.ObjectInfo {
	return nil
}

func (repo GitRepo) GetType() *coretypes.Type {
	return gitRepoType
}

func (repo GitRepo) Hash() uint32 {
	return repo.hash
}

func (repo GitRepo) WithInfo(_info *coretypes.ObjectInfo) coretypes.Object {
	return repo
}

func EnsureArgIsGitRepo(args []coretypes.Object, index int) GitRepo {
	if index < 0 || index >= len(args) {
		panic(RT.NewError("Expected GitRepo argument"))
	}
	obj := args[index]
	if c, yes := obj.(GitRepo); yes {
		return c
	}
	panic(FailArg(obj, "GitRepo", index))
}

func ExtractGitRepo(args []coretypes.Object, index int) *git.Repository {
	return EnsureArgIsGitRepo(args, index).repo
}

func open(path string) *git.Repository {
	repo, err := git.PlainOpen(path)
	PanicOnErr(err)
	return repo
}

func addUser(m *corecollections.ArrayMap, section string, name string, email string) {
	user := corecollections.EmptyArrayMap()
	user.Add(coretypes.MakeKeyword(STRINGS.Intern, "name"), coretypes.MakeString(name))
	user.Add(coretypes.MakeKeyword(STRINGS.Intern, "email"), coretypes.MakeString(email))
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, section), user)
}

func makeRemote(remote *gitConfig.RemoteConfig) coretypes.Map {
	res := corecollections.EmptyArrayMap()
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "name"), coretypes.MakeString(remote.Name))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "urls"), MakeStringVector(remote.URLs))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "mirror?"), coretypes.MakeBoolean(remote.Mirror))
	return res
}

func makeSubmodule(submodule *gitConfig.Submodule) coretypes.Map {
	res := corecollections.EmptyArrayMap()
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "name"), coretypes.MakeString(submodule.Name))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "path"), coretypes.MakeString(submodule.Path))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "url"), coretypes.MakeString(submodule.URL))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "branch"), coretypes.MakeString(submodule.Branch))
	return res
}

func makeBranch(branch *gitConfig.Branch) coretypes.Map {
	res := corecollections.EmptyArrayMap()
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "name"), coretypes.MakeString(branch.Name))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "remote"), coretypes.MakeString(branch.Remote))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "merge"), coretypes.MakeString(string(branch.Merge)))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "rebase"), coretypes.MakeString(branch.Rebase))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "description"), coretypes.MakeString(branch.Description))
	return res
}

func makeUrl(url *gitConfig.URL) coretypes.Map {
	res := corecollections.EmptyArrayMap()
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "name"), coretypes.MakeString(url.Name))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "instead-of"), coretypes.MakeString(url.InsteadOf))
	return res
}

func config(repo *git.Repository) coretypes.Map {
	cfg, err := repo.Config()
	PanicOnErr(err)
	return makeConfigMap(cfg)
}

func makeConfigMap(cfg *gitConfig.Config) coretypes.Map {
	res := corecollections.EmptyArrayMap()
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "bare?"), coretypes.MakeBoolean(cfg.Core.IsBare))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "worktree"), coretypes.MakeString(cfg.Core.Worktree))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "comment-char"), coretypes.MakeString(cfg.Core.CommentChar))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "repository-format-version"), coretypes.MakeString(string(cfg.Core.RepositoryFormatVersion)))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "default-branch"), coretypes.MakeString(cfg.Init.DefaultBranch))
	addUser(res, "user", cfg.User.Name, cfg.User.Email)
	addUser(res, "author", cfg.Author.Name, cfg.Author.Email)
	addUser(res, "committer", cfg.Committer.Name, cfg.Committer.Email)
	remotes := corecollections.EmptyArrayMap()
	for name, remote := range cfg.Remotes {
		remotes.Add(coretypes.MakeString(name), makeRemote(remote))
	}
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "remotes"), remotes)
	submodules := corecollections.EmptyArrayMap()
	for name, submodule := range cfg.Submodules {
		submodules.Add(coretypes.MakeString(name), makeSubmodule(submodule))
	}
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "submodules"), submodules)
	branches := corecollections.EmptyArrayMap()
	for name, branch := range cfg.Branches {
		branches.Add(coretypes.MakeString(name), makeBranch(branch))
	}
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "branches"), branches)
	urls := corecollections.EmptyArrayMap()
	for name, url := range cfg.URLs {
		urls.Add(coretypes.MakeString(name), makeUrl(url))
	}
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "urls"), urls)
	return res
}

func makeRef(r *plumbing.Reference) coretypes.Map {
	res := corecollections.EmptyArrayMap()
	refType := "invalid"
	if r.Type() == plumbing.HashReference {
		refType = "hash"
	} else if r.Type() == plumbing.SymbolicReference {
		refType = "symbolic"
	}
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "type"), coretypes.MakeKeyword(STRINGS.Intern, refType))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "name"), coretypes.MakeString(string(r.Name())))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "target"), coretypes.MakeString(string(r.Target())))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "hash"), coretypes.MakeString(r.Hash().String()))
	return res
}

func ref(repo *git.Repository, name string, resolved bool) coretypes.Map {
	r, err := repo.Reference(plumbing.ReferenceName(name), resolved)
	PanicOnErr(err)
	return makeRef(r)
}

func head(repo *git.Repository) coretypes.Map {
	r, err := repo.Head()
	PanicOnErr(err)
	return makeRef(r)
}

func makeSignature(s object.Signature) coretypes.Map {
	res := corecollections.EmptyArrayMap()
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "name"), coretypes.MakeString(s.Name))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "email"), coretypes.MakeString(s.Email))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "when"), coretypes.MakeTime(s.When))
	return res
}

func makeCommit(cmt *object.Commit) coretypes.Map {
	res := corecollections.EmptyArrayMap()
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "hash"), coretypes.MakeString(cmt.Hash.String()))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "author"), makeSignature(cmt.Author))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "committer"), makeSignature(cmt.Committer))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "pgp-siganture"), coretypes.MakeString(cmt.PGPSignature))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "message"), coretypes.MakeString(cmt.Message))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "tree-hash"), coretypes.MakeString(cmt.TreeHash.String()))
	parentHashes := corecollections.EmptyVector()
	for _, v := range cmt.ParentHashes {
		parentHashes = parentHashes.Conjoin(coretypes.MakeString(v.String()))
	}
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "parent-hashes"), parentHashes)
	return res
}

func log(repo *git.Repository, opts coretypes.Map) coretypes.Vec {
	var logOpts git.LogOptions
	if ok, v := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "from")); ok {
		logOpts.From = plumbing.NewHash(coretypes.EnsureObjectIsString(v, "").S)
	}
	if ok, v := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "order")); ok {
		if v.Equals(coretypes.MakeKeyword(STRINGS.Intern, "default")) {
			logOpts.Order = git.LogOrderDefault
		} else if v.Equals(coretypes.MakeKeyword(STRINGS.Intern, "dfs")) {
			logOpts.Order = git.LogOrderDFS
		} else if v.Equals(coretypes.MakeKeyword(STRINGS.Intern, "dfs-post")) {
			logOpts.Order = git.LogOrderDFSPost
		} else if v.Equals(coretypes.MakeKeyword(STRINGS.Intern, "bsf")) {
			logOpts.Order = git.LogOrderBSF
		} else if v.Equals(coretypes.MakeKeyword(STRINGS.Intern, "committer-time")) {
			logOpts.Order = git.LogOrderCommitterTime
		} else {
			panic(RT.NewError(":order must be one of: :default, :dfs, :dfs-post, :bsf, :committer-time"))
		}
	}
	if ok, v := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "path-filter")); ok {
		fn := EnsureObjectIsFn(v, "Invalid :path-filter option: %s")
		logOpts.PathFilter = func(s string) bool {
			return ToBool(fn.Call([]coretypes.Object{coretypes.MakeString(s)}))
		}
	}
	if ok, v := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "all")); ok {
		logOpts.All = ToBool(v)
	}
	if ok, v := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "since")); ok {
		t := coretypes.EnsureObjectIsTime(v, "Invalid :since option: %s").T
		logOpts.Since = &t
	}
	if ok, v := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "until")); ok {
		t := coretypes.EnsureObjectIsTime(v, "Invalid :until option: %s").T
		logOpts.Until = &t
	}
	it, err := repo.Log(&logOpts)
	PanicOnErr(err)
	res := corecollections.EmptyArrayVector()
	err = it.ForEach(func(cmt *object.Commit) error {
		res.Append(makeCommit(cmt))
		return nil
	})
	PanicOnErr(err)
	return res
}

func resolveRevision(repo *git.Repository, rev string) string {
	hash, err := repo.ResolveRevision(plumbing.Revision(rev))
	PanicOnErr(err)
	return hash.String()
}

func findCommit(repo *git.Repository, hash string) coretypes.Map {
	obj, err := repo.CommitObject(plumbing.NewHash(hash))
	PanicOnErr(err)
	return makeCommit(obj)
}

func addPath(repo *git.Repository, path string) string {
	workTree, err := repo.Worktree()
	PanicOnErr(err)
	hash, err := workTree.Add(path)
	PanicOnErr(err)
	return hash.String()
}

func addCommit(repo *git.Repository, msg string, opts coretypes.Map) string {
	workTree, err := repo.Worktree()
	PanicOnErr(err)
	var commitOpts git.CommitOptions
	if ok, v := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "all")); ok {
		commitOpts.All = ToBool(v)
	}
	if ok, v := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "allow-empty-commits")); ok {
		commitOpts.AllowEmptyCommits = ToBool(v)
	}
	hash, err := workTree.Commit(msg, &commitOpts)
	PanicOnErr(err)
	return hash.String()
}

func findObject(repo *git.Repository, hash string) coretypes.Map {
	obj, err := repo.Object(plumbing.AnyObject, plumbing.NewHash(hash))
	PanicOnErr(err)
	res := corecollections.EmptyArrayMap()
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "id"), coretypes.MakeString(obj.ID().String()))
	objType := coretypes.MakeKeyword(STRINGS.Intern, "invalid")
	switch obj.Type() {
	case 1:
		objType = coretypes.MakeKeyword(STRINGS.Intern, "commit")
	case 2:
		objType = coretypes.MakeKeyword(STRINGS.Intern, "tree")
	case 3:
		objType = coretypes.MakeKeyword(STRINGS.Intern, "blob")
	case 4:
		objType = coretypes.MakeKeyword(STRINGS.Intern, "tag")
	case 6:
		objType = coretypes.MakeKeyword(STRINGS.Intern, "ofs-delta")
	case 7:
		objType = coretypes.MakeKeyword(STRINGS.Intern, "ref-delta")
	case 8:
		objType = coretypes.MakeKeyword(STRINGS.Intern, "any")
	}
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "type"), objType)
	return res
}

func init() {
	gitRepoType = coretypes.NewValueType("GitRepo", (*GitRepo)(nil), nil)
}
