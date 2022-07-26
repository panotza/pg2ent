create
extension if not exists pgcrypto;

create table users (
    id                  bigserial,
    username            varchar     not null,
    email               varchar     not null,
    password            varchar     not null,
    password_updated_at timestamptz,
    first_name          varchar     not null default '',
    last_name           varchar     not null default '',
    role                varchar     not null default '',
    status              int         not null default 0,
    description         varchar     not null default '',
    last_login          timestamptz,
    login_failed_count  int         not null default 0,
    employee_id         bigint,
    created_by          bigint,
    created_at          timestamptz not null default now(),
    primary key (id),
    foreign key (created_by) references users (id)
);
create unique index if not exists users_username_key on users (username);

create table user_settings (
    user_id bigint,
    b       bool not null default false,
    primary key (user_id),
    foreign key (user_id) references users (id)
);

create table global_configs (
    id          bigserial   not null,
    key         varchar     not null,
    value       varchar     not null,
    description varchar     not null default '',
    created_at  timestamptz not null default now(),
    created_by  bigint,
    updated_at  timestamptz,
    updated_by  bigint,
    primary key (id)
);
create unique index if not exists global_configs_key on global_configs (key);

create table tokens (
    id         bigserial,
    code       varchar     not null,
    user_id    bigint,
    type       varchar     not null,
    created_at timestamptz not null default now(),
    primary key (id),
    foreign key (user_id) references users (id)
);
create unique index if not exists tokens_code_key on tokens (code);

create table mtm_fxs (
    id         uuid default gen_random_uuid(),
    value_date date not null,
    primary key (id)
);

create table kv_settings (
    id    varchar,
    value varchar not null,
    primary key (id)
);