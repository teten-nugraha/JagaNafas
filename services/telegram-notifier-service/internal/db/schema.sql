-- Schema from PRD section 8. CREATE TABLE IF NOT EXISTS so it's safe to run
-- on every startup (no separate migration tool in the stack).

CREATE TABLE IF NOT EXISTS users (
  id BIGSERIAL PRIMARY KEY,
  telegram_chat_id BIGINT UNIQUE NOT NULL,
  username VARCHAR,
  created_at TIMESTAMP DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sensitivity_profiles (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT REFERENCES users(id),
  condition_type VARCHAR, -- 'asma_ringan', 'asma_berat', 'ispa_berulang', 'umum'
  sensitivity_level SMALLINT, -- 1-5
  updated_at TIMESTAMP DEFAULT now()
);

CREATE TABLE IF NOT EXISTS locations (
  id BIGSERIAL PRIMARY KEY,
  city_name VARCHAR NOT NULL,
  lat DOUBLE PRECISION,
  lon DOUBLE PRECISION,
  UNIQUE(lat, lon)
);

CREATE TABLE IF NOT EXISTS user_subscriptions (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT REFERENCES users(id),
  location_id BIGINT REFERENCES locations(id),
  is_active BOOLEAN DEFAULT true,
  created_at TIMESTAMP DEFAULT now()
);

CREATE TABLE IF NOT EXISTS risk_score_history (
  id BIGSERIAL PRIMARY KEY,
  location_id BIGINT REFERENCES locations(id),
  user_id BIGINT REFERENCES users(id),
  pm25 DOUBLE PRECISION,
  pm10 DOUBLE PRECISION,
  temperature DOUBLE PRECISION,
  humidity DOUBLE PRECISION,
  risk_score DOUBLE PRECISION,
  trend VARCHAR, -- 'naik', 'turun', 'stabil'
  computed_at TIMESTAMP DEFAULT now()
);

CREATE TABLE IF NOT EXISTS alert_history (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT REFERENCES users(id),
  location_id BIGINT REFERENCES locations(id),
  risk_score DOUBLE PRECISION,
  message TEXT,
  sent_at TIMESTAMP DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_user_subscriptions_location_active
  ON user_subscriptions (location_id) WHERE is_active = true;

CREATE INDEX IF NOT EXISTS idx_risk_score_history_location_computed
  ON risk_score_history (location_id, computed_at DESC);

CREATE INDEX IF NOT EXISTS idx_risk_score_history_user_computed
  ON risk_score_history (user_id, computed_at DESC);
