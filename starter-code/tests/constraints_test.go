package tests

import (
	"testing"

	"gorm.io/gorm"

	"midterm-lab-exam/config"
	"midterm-lab-exam/models"
)

// setupTestDB เชื่อมต่อฐานข้อมูลและ AutoMigrate สำหรับการทดสอบ
// หมายเหตุ: ล้างข้อมูลในตารางที่เกี่ยวข้องก่อนเริ่มแต่ละ test case
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := config.ConnectDatabase()

	if err := db.AutoMigrate(&models.Member{}, &models.Book{}, &models.BorrowRecord{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	// Clean up tables before each test (order matters due to foreign keys)
	db.Exec("DELETE FROM borrow_records")
	db.Exec("DELETE FROM members")
	db.Exec("DELETE FROM books")

	return db
}

// ============================================================================
// ข้อที่ 3: การทดสอบและตรวจสอบการทำงานของ Database Constraints (3 คะแนน)
// ให้นักศึกษาเขียน Unit Tests เพื่อทดสอบ Constraints อย่างน้อย 5 กรณีดังต่อไปนี้:
//   1. TestCreateValidRecords         - เพิ่มข้อมูลที่ถูกต้องครบถ้วนต้องสำเร็จ (Positive Case)
//   2. TestUniqueConstraint           - ทดสอบ UNIQUE (เช่น StudentID หรือ ISBN ซ้ำต้อง Error)
//   3. TestNotNullConstraint          - ทดสอบ NOT NULL (เช่น เว้นว่าง FullName หรือ Title ต้อง Error)
//   4. TestCheckConstraint            - ทดสอบ CHECK (เช่น Stock < 0 หรือ Status ไม่ตรงตามเงื่อนไขต้อง Error)
//   5. TestForeignKeyAndDeleteRestrict - ทดสอบ FK และ OnDelete:RESTRICT
// ============================================================================

func TestCreateValidRecords(t *testing.T) {
	db := setupTestDB(t)
	_ = db
	t.Skip("TODO: implement TestCreateValidRecords")
}

func TestUniqueConstraint(t *testing.T) {
	db := setupTestDB(t)
	_ = db
	t.Skip("TODO: implement TestUniqueConstraint")
}

func TestNotNullConstraint(t *testing.T) {
	db := setupTestDB(t)
	_ = db
	t.Skip("TODO: implement TestNotNullConstraint")
}

func TestCheckConstraint(t *testing.T) {
	db := setupTestDB(t)
	_ = db
	t.Skip("TODO: implement TestCheckConstraint")
}

func TestForeignKeyAndDeleteRestrict(t *testing.T) {
	db := setupTestDB(t)
	_ = db
	t.Skip("TODO: implement TestForeignKeyAndDeleteRestrict")
}
