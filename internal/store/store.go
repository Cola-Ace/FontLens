package store

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"unicode/utf8"

	"fontlens/internal/coverage"
	"fontlens/internal/fonts"
	_ "modernc.org/sqlite"
)

const (
	legacyFaceVersion = 10
	faceVersion       = 11
	coverageVersion   = 10
)

var gzipReaders sync.Pool
var gzipWriters sync.Pool

const saveFacesSQL = `INSERT INTO faces(path,size,modified,version,payload) VALUES(?,?,?,?,?)
	ON CONFLICT(path) DO UPDATE SET size=excluded.size, modified=excluded.modified,
	version=excluded.version, payload=excluded.payload`

type Store struct{ db *sql.DB }

type cachedFaces struct {
	size, modified int64
	version        int
	payload        []byte
}

type FaceCache struct{ files map[string]cachedFaces }

type FaceRecord struct {
	Path           string
	Size, Modified int64
	Faces          []fonts.Entry
	Upgrade        bool
}

func (s *Store) SaveFacesBatch(ctx context.Context, records []FaceRecord) error {
	if len(records) == 0 {
		return nil
	}
	payloads := make([][]byte, len(records))
	for i, record := range records {
		if err := ctx.Err(); err != nil {
			return err
		}
		var err error
		payloads[i], err = encodeFaces(record.Faces)
		if err != nil {
			return err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statement, err := tx.PrepareContext(ctx, saveFacesSQL)
	if err != nil {
		return err
	}
	defer statement.Close()
	upgrade, err := tx.PrepareContext(ctx, `UPDATE faces SET version=?, payload=?
		WHERE path=? AND size=? AND modified=? AND version=?`)
	if err != nil {
		return err
	}
	defer upgrade.Close()
	for i, record := range records {
		if record.Upgrade {
			_, err = upgrade.ExecContext(ctx, faceVersion, payloads[i], record.Path, record.Size, record.Modified, legacyFaceVersion)
		} else {
			_, err = statement.ExecContext(ctx, record.Path, record.Size, record.Modified, faceVersion, payloads[i])
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) LoadFaces(ctx context.Context) (*FaceCache, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT path,size,modified,version,payload FROM faces WHERE version IN (?,?)`, legacyFaceVersion, faceVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cache := &FaceCache{files: make(map[string]cachedFaces)}
	for rows.Next() {
		var path string
		var file cachedFaces
		if err := rows.Scan(&path, &file.size, &file.modified, &file.version, &file.payload); err != nil {
			return nil, err
		}
		cache.files[path] = file
	}
	return cache, rows.Err()
}

func (c *FaceCache) Faces(path string, size, modified int64) ([]fonts.Entry, bool) {
	record := FaceRecord{Path: path, Size: size, Modified: modified}
	found := c.Lookup(&record)
	return record.Faces, found
}

func (c *FaceCache) Lookup(record *FaceRecord) bool {
	file, found := c.files[record.Path]
	if !found || file.size != record.Size || file.modified != record.Modified {
		return false
	}
	record.Faces, found = decodeFaces(file.payload)
	record.Upgrade = found && file.version == legacyFaceVersion
	return found
}

func encodeFaces(faces []fonts.Entry) ([]byte, error) {
	for _, face := range faces {
		if !utf8.ValidString(face.Path) || !utf8.ValidString(face.Format) || !utf8.ValidString(face.FullName) || !utf8.ValidString(face.Family) {
			return encode(faces)
		}
	}
	return json.Marshal(faces)
}

func decodeFaces(payload []byte) ([]fonts.Entry, bool) {
	var faces []fonts.Entry
	var err error
	if bytes.HasPrefix(payload, []byte{0x1f, 0x8b}) {
		err = decode(payload, &faces)
	} else {
		err = json.Unmarshal(payload, &faces)
	}
	if err != nil {
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
	err := s.db.QueryRow(`SELECT payload FROM faces WHERE path=? AND size=? AND modified=? AND version IN (?,?)`, path, size, modified, legacyFaceVersion, faceVersion).Scan(&payload)
	if err != nil {
		return nil, false
	}
	return decodeFaces(payload)
}

func (s *Store) SaveFaces(path string, size, modified int64, faces []fonts.Entry) error {
	payload, err := encodeFaces(faces)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(saveFacesSQL, path, size, modified, faceVersion, payload)
	return err
}

func (s *Store) Coverage(key string) (coverage.Result, bool) {
	var payload []byte
	err := s.db.QueryRow(`SELECT payload FROM coverage WHERE key=? AND version=?`, key, coverageVersion).Scan(&payload)
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
		ON CONFLICT(key) DO UPDATE SET version=excluded.version, payload=excluded.payload`, key, coverageVersion, payload)
	return err
}

func encode(value any) ([]byte, error) {
	var buffer bytes.Buffer
	zip, _ := gzipWriters.Get().(*gzip.Writer)
	if zip == nil {
		var err error
		zip, err = gzip.NewWriterLevel(&buffer, gzip.BestSpeed)
		if err != nil {
			return nil, err
		}
	} else {
		zip.Reset(&buffer)
	}
	defer gzipWriters.Put(zip)
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
