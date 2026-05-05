-- +goose Up 
CREATE TABLE chirps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    body TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE
);

-- +goose Down  
DROP TABLE chirps;