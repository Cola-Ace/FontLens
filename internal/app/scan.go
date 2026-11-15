package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"fontlens/internal/fonts"
	"fontlens/internal/store"
)

func (a *appState) readFaces(path string, cached *store.FaceCache) ([]fonts.Entry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if cached != nil {
		if faces, found := cached.Faces(path, info.Size(), info.ModTime().UnixNano()); found {
			return faces, nil
		}
	} else if a.store != nil {
		if faces, found := a.store.Faces(path, info.Size(), info.ModTime().UnixNano()); found {
			return faces, nil
		}
	}
	faces, err := fonts.ReadIndex(path)
	if err == nil && a.store != nil {
		_ = a.store.SaveFaces(path, info.Size(), info.ModTime().UnixNano(), faces)
	}
	return faces, err
}

func systemSources() []string {
	windows := os.Getenv("WINDIR")
	if windows == "" {
		windows = `C:\Windows`
	}
	roots := []string{filepath.Join(windows, "Fonts")}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		roots = append(roots, filepath.Join(local, "Microsoft", "Windows", "Fonts"))
	}
	return roots
}

func (a *appState) refresh() {
	if a.cancelScan != nil {
		a.cancelScan()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancelScan = cancel
	a.scanID++
	current := a.scanID
	a.scanning = true
	if a.faceIndex == nil {
		a.faceIndex = make(map[string]int)
	}
	a.dirs = make(map[string][]string)
	a.errors = nil
	a.systemIDs = make(map[string]bool)
	a.recount()
	roots := slices.Clone(a.sources)
	go func() {
		var cached *store.FaceCache
		if a.store != nil {
			cached, _ = a.store.LoadFaces(ctx)
		}
		readFaces := func(path string) ([]fonts.Entry, error) {
			return a.readFaces(path, cached)
		}
		seen := make(map[string]struct{})
		type scanFace struct {
			face   fonts.Entry
			system bool
		}
		batch := make([]scanFace, 0, 512)
		var directories, messages []string
		lastFlush := time.Now()
		var lastRebuild time.Time
		flush := func() {
			if len(batch)+len(directories)+len(messages) == 0 || ctx.Err() != nil {
				return
			}
			ready, dirs, errors := batch, directories, messages
			done := make(chan struct{})
			a.window.Update(func() {
				defer close(done)
				if current != a.scanID || ctx.Err() != nil {
					return
				}
				for _, item := range ready {
					face := item.face
					if item.system && !a.systemIDs[face.ID()] {
						a.systemIDs[face.ID()] = true
						if _, exists := a.faceIndex[face.ID()]; exists {
							a.addScopeCount("system", fontNameKey(face))
						}
					}
					if index, exists := a.faceIndex[face.ID()]; exists {
						a.faces[index] = face
					} else {
						a.faceIndex[face.ID()] = len(a.faces)
						a.faces = append(a.faces, face)
						a.addCount(face)
					}
				}
				for _, path := range dirs {
					parent := filepath.Dir(path)
					if !slices.Contains(a.dirs[parent], path) {
						a.dirs[parent] = append(a.dirs[parent], path)
					}
				}
				a.errors = append(a.errors, errors...)
				if len(ready) > 0 && time.Since(lastRebuild) >= 250*time.Millisecond {
					a.rebuild()
					lastRebuild = time.Now()
				}
			})
			// Keep at most one scan update queued so cached reads cannot flood the UI thread.
			select {
			case <-done:
				batch = batch[:0]
				directories, messages = directories[:0], messages[:0]
				lastFlush = time.Now()
			case <-ctx.Done():
			}
		}
		flushIfReady := func() {
			if len(batch)+len(directories)+len(messages) >= cap(batch) || time.Since(lastFlush) >= 100*time.Millisecond {
				flush()
			}
		}
		enqueue := func(face fonts.Entry, system bool) {
			id := face.ID()
			if _, exists := seen[id]; exists {
				return
			}
			seen[id] = struct{}{}
			batch = append(batch, scanFace{face: face, system: system})
			flushIfReady()
		}
		report := func(message string) {
			messages = append(messages, message)
			flushIfReady()
		}
		wanted := make(map[string]map[int]bool)
		if err := fonts.SystemFaces(ctx, func(path string, index int) {
			if wanted[path] == nil {
				wanted[path] = make(map[int]bool)
			}
			wanted[path][index] = true
		}, func(err error) { report(err.Error()) }); err != nil && ctx.Err() == nil {
			report("DirectWrite 系统字体枚举: " + err.Error())
		}
		paths := make([]string, 0, len(wanted))
		for path := range wanted {
			paths = append(paths, path)
		}
		slices.SortFunc(paths, func(left, right string) int { return strings.Compare(strings.ToLower(left), strings.ToLower(right)) })
		for _, path := range paths {
			if ctx.Err() != nil {
				break
			}
			faces, err := readFaces(path)
			if err != nil {
				report(fmt.Sprintf("%s: %v", path, err))
				continue
			}
			for _, face := range faces {
				if wanted[path][face.Index] {
					enqueue(face, true)
				}
			}
		}
		flush()
		for _, root := range roots {
			if ctx.Err() != nil {
				break
			}
			if _, err := os.Stat(root); err != nil {
				continue
			}
			_ = fonts.Scan(ctx, root, readFaces, func(path string) {
				if path == root {
					return
				}
				directories = append(directories, path)
				flushIfReady()
			}, func(face fonts.Entry) {
				enqueue(face, false)
			}, func(path string, err error) {
				report(fmt.Sprintf("%s: %v", path, err))
			})
			flush()
		}
		a.window.Update(func() {
			if current == a.scanID {
				if ctx.Err() == nil {
					a.faces = slices.DeleteFunc(a.faces, func(face fonts.Entry) bool {
						_, exists := seen[face.ID()]
						return !exists
					})
					a.faceIndex = make(map[string]int, len(a.faces))
					for index, face := range a.faces {
						a.faceIndex[face.ID()] = index
					}
					a.compareIDs = slices.DeleteFunc(a.compareIDs, func(id string) bool {
						_, exists := a.faceIndex[id]
						return !exists
					})
					if len(a.compareIDs) < 4 {
						a.compareLimit = false
					}
					a.recount()
				}
				a.scanning = false
				a.cancelScan = nil
				a.rebuild()
			}
		})
	}()
}
