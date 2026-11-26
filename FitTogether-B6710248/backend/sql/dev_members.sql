CREATE TABLE IF NOT EXISTS users (
    id bigint PRIMARY KEY,
    email text UNIQUE NOT NULL,
    is_email_verified boolean NOT NULL DEFAULT true,
    account_status text NOT NULL DEFAULT 'active'
);
CREATE TABLE IF NOT EXISTS profiles (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id bigint UNIQUE NOT NULL REFERENCES users(id),
    display_name text NOT NULL,
    photo_url text
);
INSERT INTO users(id,email) VALUES
    (1,'tom@g.sut.ac.th'),(2,'mint@g.sut.ac.th'),(3,'beam@g.sut.ac.th')
ON CONFLICT DO NOTHING;
INSERT INTO profiles(user_id,display_name) VALUES (1,'Tom'),(2,'Mint'),(3,'Beam') ON CONFLICT DO NOTHING;
