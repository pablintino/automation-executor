package models

import (
	"github.com/google/uuid"
)

const SecretTypeIdCredentialsTupleId = 2
const SecretTypeIdKeyTokenId = 1
const SecretTypeIdRegistryTupleId = 3

type SecretModel struct {
	SecretId uuid.UUID `db:"secret_id"`
	Name     string    `db:"name"`
	TypeId   int       `db:"type_id"`
	Key      string    `db:"secret_key"`
	Value    string    `db:"secret_value"`
}

type RegistrySecretModel struct {
	SecretModel
	RegistrySecretId uuid.UUID `db:"registry_secret_id"`
	Registry         string    `db:"registry"`
}
