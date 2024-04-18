package models

type TaskModel struct {
	Id     string `db:"id"`
	Name   string `db:"name"`
	TypeId int    `db:"type_id"`
}

type AnsibleTaskModel struct {
	TaskModel
	TaskId              string      `db:"task_id"`
	Playbook            string      `db:"playbook"`
	PlaybookOutPatterns StringArray `db:"playbook_out_patterns"`
	PlaybookVars        StringMap   `db:"playbook_vars"`
}

type ScriptTaskModel struct {
	TaskModel
}
