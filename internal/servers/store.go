package servers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

type Server struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Trusted  bool   `json:"trusted"`
}

type Record struct {
	Server
	Password string
	HostKey  []byte
}

type Store struct {
	db     *sql.DB
	secret string
}

func New(db *sql.DB, secret string) *Store {
	return &Store{db: db, secret: secret}
}

func (s *Store) List() ([]Server, error) {
	rows, err := s.db.Query(`SELECT id, name, host, port, username, length(host_key) > 0 FROM servers ORDER BY name, host`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Server{}
	for rows.Next() {
		var item Server
		if err := rows.Scan(&item.ID, &item.Name, &item.Host, &item.Port, &item.Username, &item.Trusted); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) Get(id string) (Record, error) {
	var rec Record
	var sealed, hostKey []byte
	err := s.db.QueryRow(`SELECT id, name, host, port, username, password, host_key FROM servers WHERE id = ?`, id).
		Scan(&rec.ID, &rec.Name, &rec.Host, &rec.Port, &rec.Username, &sealed, &hostKey)
	if err == sql.ErrNoRows {
		return Record{}, fmt.Errorf("server was not found")
	}
	if err != nil {
		return Record{}, err
	}
	pass, err := open(s.secret, sealed)
	if err != nil {
		return Record{}, err
	}
	rec.Password = string(pass)
	rec.HostKey = hostKey
	rec.Trusted = len(hostKey) > 0
	return rec, nil
}

func (s *Store) Create(name, host string, port int, user, password string) (Server, error) {
	sealed, err := seal(s.secret, []byte(password))
	if err != nil {
		return Server{}, err
	}
	id, err := newID()
	if err != nil {
		return Server{}, err
	}
	now := time.Now().Unix()
	_, err = s.db.Exec(`INSERT INTO servers(id, name, host, port, username, password, host_key, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, '', ?, ?)`,
		id, name, host, port, user, sealed, now, now)
	if err != nil {
		return Server{}, err
	}
	return Server{ID: id, Name: name, Host: host, Port: port, Username: user}, nil
}

func (s *Store) Update(id, name, host string, port int, user, password string) (Server, error) {
	current, err := s.Get(id)
	if err != nil {
		return Server{}, err
	}
	sealedPass := password
	var sealed []byte
	if sealedPass == "" {
		err = s.db.QueryRow(`SELECT password FROM servers WHERE id = ?`, id).Scan(&sealed)
		if err != nil {
			return Server{}, err
		}
	} else {
		sealed, err = seal(s.secret, []byte(password))
		if err != nil {
			return Server{}, err
		}
	}
	hostKey := current.HostKey
	if host != current.Host || port != current.Port {
		hostKey = []byte{}
	}
	now := time.Now().Unix()
	res, err := s.db.Exec(`UPDATE servers SET name = ?, host = ?, port = ?, username = ?, password = ?, host_key = ?, updated_at = ? WHERE id = ?`,
		name, host, port, user, sealed, hostKey, now, id)
	if err != nil {
		return Server{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Server{}, fmt.Errorf("server was not found")
	}
	return Server{ID: id, Name: name, Host: host, Port: port, Username: user, Trusted: len(hostKey) > 0}, nil
}

func (s *Store) Delete(id string) error {
	res, err := s.db.Exec(`DELETE FROM servers WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("server was not found")
	}
	return nil
}

func (s *Store) SetHostKey(id string, key []byte) error {
	res, err := s.db.Exec(`UPDATE servers SET host_key = ?, updated_at = ? WHERE id = ?`, key, time.Now().Unix(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("server was not found")
	}
	return nil
}

func newID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
