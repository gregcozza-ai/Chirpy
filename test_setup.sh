#!/bin/bash
# Create test database
psql -U postgres -c "CREATE DATABASE chirpy_test;"
psql -U postgres -d chirpy_test -c "CREATE TABLE users (id UUID PRIMARY KEY, email TEXT NOT NULL, hashed_password TEXT NOT NULL);"
psql -U postgres -d chirpy_test -c "CREATE TABLE chirps (id UUID PRIMARY KEY, body TEXT NOT NULL, user_id UUID NOT NULL);"