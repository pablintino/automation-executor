package models

import (
	"github.com/google/uuid"
)

const SecretTypeIdCredentialsTupleId = 2
const SecretTypeIdKeyTokenId = 1
const SecretTypeIdRegistryTupleId = 3

type SecretModel struct {
	Id     uuid.UUID `db:"id"`
	Name   string    `db:"name"`
	TypeId int       `db:"type_id"`
	Key    string    `db:"secret_key"`
	Value  string    `db:"secret_value"`
}

type RegistrySecretModel struct {
	SecretModel
	Registry string `db:"registry"`
}
