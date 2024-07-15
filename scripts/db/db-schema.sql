create extension if not exists pgcrypto;

create table task_type
(
    id   smallint not null
        constraint task_types_pk
            primary key,
    name varchar  not null
        constraint task_types_unique_pk
            unique
);

create table environment_type
(
    id   smallint not null
        constraint environment_types_pk
            primary key,
    name varchar  not null
        constraint environment_types_unique_pk
            unique
);

create table environment
(
    id      uuid default gen_random_uuid() not null
        constraint environment_pk
            primary key,
    type_id integer                        not null
        constraint environment_types_id_fk
            references environment_type,
    state   varchar,
    constraint environment_id_type_id_unique_pk
        unique (id, type_id)
);

create table task
(
    id             uuid default gen_random_uuid() not null
        constraint task_pk
            primary key,
    name           varchar,
    type_id        smallint                       not null
        constraint task_types_id_fk
            references task_type,
    environment_id uuid
        constraint task_environment_id_fk
            references environment,
    constraint task_id_type_id_unique_pk
        unique (id, type_id)
);

create table task_ansible
(
    id                    uuid     default gen_random_uuid() not null
        constraint task_ansible_pk
            primary key,
    task_id               uuid                               not null,
    type_id               smallint default 1                 not null,
    playbook_vars         jsonb,
    playbook              varchar                            not null,
    playbook_out_patterns text[],
    constraint task_ansible_task_ansible__fk
        foreign key (task_id, type_id) references task (id, type_id)
);

create table secret_type
(
    id   smallint not null
        constraint secret_type_pk
            primary key,
    name varchar  not null
        constraint secret_type_unique_pk
            unique
);

insert into secret_type (id, name)
values (1, 'key-token'),
       (2, 'credetials-tuple'),
       (3, 'registry-tuple');

create table secret
(
    id           uuid default gen_random_uuid() not null
        constraint id
            primary key,
    name         varchar                        not null,
    type_id      smallint
        constraint secret_secret_type_id_fk
            references secret_type,
    secret_key   bytea                          not null,
    secret_value bytea
);

create table secret_registry
(
    id        uuid     default gen_random_uuid() not null
        constraint secret_registries_pk
            primary key,
    registry  varchar                            not null
        constraint secret_registries_unique_pk
            unique,
    secret_id uuid                               not null
        constraint secret_registries_secret_id_fk
            references secret
            on delete cascade,
    type_id   smallint default 3                 not null
        constraint secret_registries_secret_type_id_fk
            references secret_type
);
