package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"fontlens/internal/fonts"
)

func (a *appState) readFaces(path string) ([]fonts.Entry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if a.store != nil {
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
		seen := make(map[string]struct{})
		type scanFace struct {
			face   fonts.Entry
			system bool
		}
		batch := make([]scanFace, 0, 32)
		flush := func() {
			if len(batch) == 0 {
				return
			}
			ready := append([]scanFace(nil), batch...)
			batch = batch[:0]
			a.window.Update(func() {
				if current != a.scanID {
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
				a.rebuild()
			})
		}
		enqueue := func(face fonts.Entry, system bool) {
			id := face.ID()
			if _, exists := seen[id]; exists {
				return
			}
			seen[id] = struct{}{}
			batch = append(batch, scanFace{face: face, system: system})
			if len(batch) == cap(batch) {
				flush()
			}
		}
		report := func(message string) {
			a.window.Update(func() {
				if current == a.scanID {
					a.errors = append(a.errors, message)
				}
			})
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
			faces, err := a.readFaces(path)
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
			_ = fonts.Scan(ctx, root, a.readFaces, func(path string) {
				if path == root {
					return
				}
				a.window.Update(func() {
					if current != a.scanID {
						return
					}
					parent := filepath.Dir(path)
					if !slices.Contains(a.dirs[parent], path) {
						a.dirs[parent] = append(a.dirs[parent], path)
					}
				})
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
