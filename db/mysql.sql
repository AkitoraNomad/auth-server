DROP TABLE IF EXISTS auth_user_provider;
DROP TABLE IF EXISTS auth_client;
DROP TABLE IF EXISTS auth_admin;
DROP TABLE IF EXISTS auth_user;

CREATE TABLE auth_user
(
    id            bigint NOT NULL AUTO_INCREMENT,
    email         VARCHAR(255),
    display_name  VARCHAR(255),
    created_at    DATETIME,
    updated_at    DATETIME,
    last_login_at DATETIME,
    PRIMARY KEY (id)
);

CREATE TABLE auth_user_provider
(
    id            bigint       NOT NULL AUTO_INCREMENT,
    user_id       bigint       not null,
    login_key     VARCHAR(255) not null,
    provider_type VARCHAR(255) not null,
    provider_data json,
    created_at    DATETIME,
    updated_at    DATETIME,
    PRIMARY KEY (id)
);

CREATE TABLE auth_client
(
    id           VARCHAR(255) NOT NULL,
    display_name VARCHAR(255),
    secret       text,
    domain       text,
    scopes       json,
    token_type   VARCHAR(255),
    created_at   DATETIME,
    updated_at   DATETIME,
    PRIMARY KEY (id)
);

CREATE TABLE auth_admin
(
    id         bigint NOT NULL AUTO_INCREMENT,
    user_id    bigint NOT NULL,
    created_at DATETIME,
    updated_at DATETIME,
    PRIMARY KEY (id)
);
