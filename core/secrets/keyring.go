package secrets

import (
	"errors"

	"github.com/zalando/go-keyring"
)

const service = "postacontrol"

// NewKeyring devuelve un Store respaldado por el llavero del sistema
// (GNOME Keyring vía Secret Service en Linux).
func NewKeyring() Store {
	return keyringStore{}
}

type keyringStore struct{}

func (keyringStore) Set(email, password string) error {
	return keyring.Set(service, email, password)
}

func (keyringStore) Get(email string) (string, error) {
	pw, err := keyring.Get(service, email)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return pw, err
}

func (keyringStore) Delete(email string) error {
	err := keyring.Delete(service, email)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
