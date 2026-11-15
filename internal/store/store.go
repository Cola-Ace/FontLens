package store

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"fontlens/internal/coverage"
	"fontlens/internal/fonts"
	_ "modernc.org/sqlite"
)

const version = 10

var gzipReaders sync.Pool

type Store struct{ db *sql.DB }

type cachedFaces struct {
	size, modified int64
	payload        []byte
}

type FaceCache struct{ files map[string]cachedFaces }

func (s *Store) LoadFaces(ctx context.Context) (*FaceCache, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT path,size,modified,payload FROM faces WHERE version=?`, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cache := &FaceCache{files: make(map[string]cachedFaces)}
	for rows.Next() {
		var path string
		var file cachedFaces
		if err := rows.Scan(&path, &file.size, &file.modified, &file.payload); err != nil {
			return nil, err
		}
		cache.files[path] = file
	}
	return cache, rows.Err()
}

func (c *FaceCache) Faces(path string, size, modified int64) ([]fonts.Entry, bool) {
	file, found := c.files[path]
	if !found || file.size != size || file.modified != modified {
		return nil, false
	}
	var faces []fonts.Entry
	if decode(file.payload, &faces) != nil {
		return nil, false
	}
	return faces, true
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS faces (
		path TEXT PRIMARY KEY, size INTEGER NOT NULL, modified INTEGER NOT NULL,
		version INTEGER NOT NULL, payload BLOB NOT NULL
	)`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS coverage (
		key TEXT PRIMARY KEY, version INTEGER NOT NULL, payload BLOB NOT NULL
	)`); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Faces(path string, size, modified int64) ([]fonts.Entry, bool) {
	var payload []byte
	err := s.db.QueryRow(`SELECT payload FROM faces WHERE path=? AND size=? AND modified=? AND version=?`, path, size, modified, version).Scan(&payload)
	if err != nil {
		return nil, false
	}
	var faces []fonts.Entry
	if decode(payload, &faces) != nil {
		return nil, false
	}
	return faces, true
}

func (s *Store) SaveFaces(path string, size, modified int64, faces []fonts.Entry) error {
	payload, err := encode(faces)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO faces(path,size,modified,version,payload) VALUES(?,?,?,?,?)
		ON CONFLICT(path) DO UPDATE SET size=excluded.size, modified=excluded.modified,
		version=excluded.version, payload=excluded.payload`, path, size, modified, version, payload)
	return err
}

func (s *Store) Coverage(key string) (coverage.Result, bool) {
	var payload []byte
	err := s.db.QueryRow(`SELECT payload FROM coverage WHERE key=? AND version=?`, key, version).Scan(&payload)
	if err != nil {
		return coverage.Result{}, false
	}
	var result coverage.Result
	if decode(payload, &result) != nil {
		return coverage.Result{}, false
	}
	return result, true
}

func (s *Store) SaveCoverage(key string, result coverage.Result) error {
	payload, err := encode(result)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO coverage(key,version,payload) VALUES(?,?,?)
		ON CONFLICT(key) DO UPDATE SET version=excluded.version, payload=excluded.payload`, key, version, payload)
	return err
}

func encode(value any) ([]byte, error) {
	var buffer bytes.Buffer
	zip, err := gzip.NewWriterLevel(&buffer, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}
	if err := gob.NewEncoder(zip).Encode(value); err != nil {
		zip.Close()
		return nil, err
	}
	if err := zip.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func decode(payload []byte, value any) error {
	zip, _ := gzipReaders.Get().(*gzip.Reader)
	var err error
	if zip == nil {
		zip, err = gzip.NewReader(bytes.NewReader(payload))
	} else {
		err = zip.Reset(bytes.NewReader(payload))
	}
	if err != nil {
		return err
	}
	defer func() {
		zip.Close()
		gzipReaders.Put(zip)
	}()
	if err := gob.NewDecoder(zip).Decode(value); err != nil {
		return fmt.Errorf("cache decode: %w", err)
	}
	return nil
}
