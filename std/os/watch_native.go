package os

import (
	"fmt"
	stdos "os"
	"path/filepath"
	"sync"

	corert "github.com/rcarmo/go-joker/core/runtime"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"

	"github.com/fsnotify/fsnotify"

	. "github.com/rcarmo/go-joker/core"
)

type fileWatcher struct {
	watcher *fsnotify.Watcher
	ch      *corert.ObjectChannel

	recursive bool
	done      chan struct{}

	closeOnce  sync.Once
	cancelOnce sync.Once
}

func watch(paths coretypes.Seqable, ch *corert.ObjectChannel, opts coretypes.Map) coretypes.Object {
	recursive := false
	if ok, obj := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "recursive?")); ok {
		recursive = coretypes.EnsureObjectIsBoolean(obj, "recursive?: %s").B
	}

	watcher, err := fsnotify.NewWatcher()
	corert.PanicOnErr(err)

	fw := &fileWatcher{
		watcher:   watcher,
		ch:        ch,
		recursive: recursive,
		done:      make(chan struct{}),
	}

	if err := fw.addPaths(paths); err != nil {
		fw.closeWatcher()
		corert.PanicOnErr(err)
	}

	go fw.run()

	return Proc{
		Fn: func(args []coretypes.Object) coretypes.Object {
			CheckArity(args, 0, 0)
			fw.cancel()
			return NIL
		},
		Name:    "watch-cancel",
		Package: "std/os",
	}
}

func (fw *fileWatcher) cancel() {
	fw.cancelOnce.Do(func() {
		fw.closeWatcher()
		fw.ch.Close()
		<-fw.done
	})
}

func (fw *fileWatcher) closeWatcher() {
	fw.closeOnce.Do(func() {
		if err := fw.watcher.Close(); err != nil {
			fmt.Fprintln(Stderr, "watch close error:", err)
		}
	})
}

func (fw *fileWatcher) addPaths(paths coretypes.Seqable) error {
	for s := paths.Seq(); !s.IsEmpty(); s = s.Rest() {
		path := coretypes.EnsureObjectIsString(s.First(), "watch path: %s").S
		if err := fw.addPath(path); err != nil {
			return err
		}
	}
	return nil
}

func (fw *fileWatcher) addPath(path string) error {
	if !fw.recursive {
		return fw.watcher.Add(path)
	}

	info, err := stdos.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fw.watcher.Add(path)
	}
	return filepath.WalkDir(path, func(path string, d stdos.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return fw.watcher.Add(path)
		}
		return nil
	})
}

func (fw *fileWatcher) run() {
	defer close(fw.done)

	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			if fw.recursive && event.Op&fsnotify.Create != 0 {
				fw.addCreatedDir(event.Name)
			}
			if !fw.send(watchEvent(event)) {
				fw.closeWatcher()
				return
			}
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			if !fw.send(watchError(err)) {
				fw.closeWatcher()
				return
			}
		}
	}
}

func (fw *fileWatcher) addCreatedDir(path string) {
	info, err := stdos.Stat(path)
	if err != nil || !info.IsDir() {
		return
	}
	if err := filepath.WalkDir(path, func(path string, d stdos.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return fw.watcher.Add(path)
		}
		return nil
	}); err != nil {
		fw.send(watchError(err))
	}
}

func (fw *fileWatcher) send(obj coretypes.Object) bool {
	return fw.ch.Send(obj)
}

func watchEvent(event fsnotify.Event) coretypes.Object {
	m := corecollections.EmptyArrayMap()
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "type"), coretypes.MakeKeyword(STRINGS.Intern, "event"))
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "path"), coretypes.MakeString(event.Name))
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "ops"), watchOps(event.Op))
	return m
}

func watchError(err error) coretypes.Object {
	m := corecollections.EmptyArrayMap()
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "type"), coretypes.MakeKeyword(STRINGS.Intern, "error"))
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "error"), RT.NewError(err.Error()))
	return m
}

func watchOps(op fsnotify.Op) *corecollections.MapSet {
	ops := corecollections.EmptySet()
	if op&fsnotify.Create != 0 {
		ops.Add(coretypes.MakeKeyword(STRINGS.Intern, "create"))
	}
	if op&fsnotify.Write != 0 {
		ops.Add(coretypes.MakeKeyword(STRINGS.Intern, "write"))
	}
	if op&fsnotify.Remove != 0 {
		ops.Add(coretypes.MakeKeyword(STRINGS.Intern, "remove"))
	}
	if op&fsnotify.Rename != 0 {
		ops.Add(coretypes.MakeKeyword(STRINGS.Intern, "rename"))
	}
	if op&fsnotify.Chmod != 0 {
		ops.Add(coretypes.MakeKeyword(STRINGS.Intern, "chmod"))
	}
	return ops
}
