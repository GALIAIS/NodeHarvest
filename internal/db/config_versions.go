package db

import "github.com/GALIAIS/NodeHarvest/internal/timex"

type ConfigVersion struct {
	ID        string `json:"id"`
	Actor     string `json:"actor"`
	Checksum  string `json:"checksum"`
	PatchJSON string `json:"patch_json"`
	CreatedAt string `json:"created_at"`
}

func (s *Store) SaveConfigVersion(version ConfigVersion) error {
	if version.CreatedAt == "" {
		version.CreatedAt = timex.NowRFC3339()
	}
	_, err := s.exec(`INSERT INTO config_versions(id,actor,checksum,config_yaml,created_at)
 VALUES(?,?,?,?,?)`, version.ID, version.Actor, version.Checksum, version.PatchJSON, version.CreatedAt)
	return err
}

func (s *Store) LatestConfigVersion() (*ConfigVersion, error) {
	row := s.queryRow(`SELECT id,actor,checksum,config_yaml,created_at
 FROM config_versions ORDER BY created_at DESC,id DESC LIMIT 1`)
	var version ConfigVersion
	if err := row.Scan(&version.ID, &version.Actor, &version.Checksum, &version.PatchJSON, &version.CreatedAt); err != nil {
		return nil, err
	}
	return &version, nil
}

func (s *Store) ListConfigVersions(limit int) ([]ConfigVersion, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.query(`SELECT id,actor,checksum,config_yaml,created_at
 FROM config_versions ORDER BY created_at DESC,id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var versions []ConfigVersion
	for rows.Next() {
		var version ConfigVersion
		if err := rows.Scan(&version.ID, &version.Actor, &version.Checksum, &version.PatchJSON, &version.CreatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}
