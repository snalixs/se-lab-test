-- ============================================================================
-- ข้อที่ 1: การสร้างตารางและกำหนด Constraints ด้วย SQL บน PostgreSQL (3 คะแนน)
-- ให้นักศึกษาเขียนคำสั่ง SQL เพื่อสร้างตาราง publishers และ books ตามข้อกำหนดในโจทย์
-- ============================================================================

-- TODO: 1.1 สร้างตาราง publishers
-- คอลัมน์ที่ต้องการ:
--   id          SERIAL PRIMARY KEY
--   name        VARCHAR(100) NOT NULL UNIQUE
--   country     VARCHAR(50) NOT NULL DEFAULT 'Thailand'
--   created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP


-- TODO: 1.2 สร้างตาราง books
-- คอลัมน์ที่ต้องการ:
--   id             SERIAL PRIMARY KEY
--   isbn           VARCHAR(13) NOT NULL UNIQUE
--   title          VARCHAR(200) NOT NULL
--   published_year INT NOT NULL CHECK (published_year BETWEEN 1900 AND 2026)
--   price          NUMERIC(10, 2) NOT NULL CHECK (price > 0)
--   stock          INT NOT NULL DEFAULT 0 CHECK (stock >= 0)
--   publisher_id   INT NOT NULL REFERENCES publishers(id) ON UPDATE CASCADE ON DELETE RESTRICT
--   created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
