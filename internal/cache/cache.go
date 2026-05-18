package cache

import (
	"encoding/json"
	"fmt"
	"time"

	"CuckoosVision/internal/model"

	"go.etcd.io/bbolt"
)

var (
	domainsBucket  = []byte("domains")
	findingsBucket = []byte("findings")
)

type Store struct {
	db  *bbolt.DB
	ttl time.Duration
}

func Open(path string, ttl time.Duration) (*Store, error) {
	db, err := bbolt.Open(path, 0600, &bbolt.Options{
		Timeout: time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("open cache: %w", err)
	}

	err = db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(domainsBucket); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists(findingsBucket)
		return err
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create buckets: %w", err)
	}

	return &Store{db: db, ttl: ttl}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) GetDomain(d string) (model.DomainStatus, bool) {
	var status model.DomainStatus
	err := s.db.View(func(tx *bbolt.Tx) error {
		v := tx.Bucket(domainsBucket).Get([]byte(d))
		if v == nil {
			return fmt.Errorf("not found")
		}
		return json.Unmarshal(v, &status)
	})
	if err != nil {
		return model.DomainStatus{}, false
	}
	if time.Since(status.CheckedAt) > s.ttl {
		return model.DomainStatus{}, false
	}
	return status, true
}

func (s *Store) PutDomain(status model.DomainStatus) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		data, err := json.Marshal(status)
		if err != nil {
			return err
		}
		return tx.Bucket(domainsBucket).Put([]byte(status.Domain), data)
	})
}

func (s *Store) AddFinding(f model.Finding) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(findingsBucket)
		existing := b.Get([]byte(f.Domain))

		if existing != nil {
			var prev model.Finding
			if err := json.Unmarshal(existing, &prev); err == nil {
				seen := make(map[string]struct{})
				for _, ref := range prev.References {
					seen[ref.SourceURL+"|"+ref.TargetURL] = struct{}{}
				}
				for _, ref := range f.References {
					key := ref.SourceURL + "|" + ref.TargetURL
					if _, ok := seen[key]; !ok {
						prev.References = append(prev.References, ref)
					}
				}
				if f.FirstSeen.Before(prev.FirstSeen) {
					prev.FirstSeen = f.FirstSeen
				}
				if f.RiskLevel < prev.RiskLevel {
					prev.RiskLevel = f.RiskLevel
				}
				f = prev
			}
		}

		data, err := json.Marshal(f)
		if err != nil {
			return err
		}
		return b.Put([]byte(f.Domain), data)
	})
}

func (s *Store) GetFindings() ([]model.Finding, error) {
	var findings []model.Finding
	err := s.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(findingsBucket).ForEach(func(k, v []byte) error {
			var f model.Finding
			if err := json.Unmarshal(v, &f); err != nil {
				return err
			}
			findings = append(findings, f)
			return nil
		})
	})
	return findings, err
}
