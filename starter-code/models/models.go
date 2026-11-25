package models

import (
	"time"

	"gorm.io/gorm"
)

// ============================================================================
// ข้อที่ 2: การสร้างโครงสร้างฐานข้อมูลด้วย GORM และภาษา Go (4 คะแนน)
// ให้นักศึกษากำหนด struct fields, GORM tags และ relationships ให้ครบถ้วนตามโจทย์
// ============================================================================

// Member แทนข้อมูลสมาชิกห้องสมุด (ตาราง members)
type Member struct {
	gorm.Model
	// TODO: กำหนดฟิลด์และ GORM tags ตามข้อกำหนดในโจทย์
	// - StudentID     (string)  : uniqueIndex, not null, size:10
	// - FullName      (string)  : not null, size:100
	// - Email         (string)  : uniqueIndex, not null, size:100
	// - Status        (string)  : not null, default:'ACTIVE', check:status IN ('ACTIVE','SUSPENDED')
	// - BorrowRecords ([]BorrowRecord) : One-to-Many (foreignKey:MemberID)
}

// Book แทนข้อมูลหนังสือ (เชื่อมโยงกับตาราง books หรือสร้างผ่าน AutoMigrate)
type Book struct {
	ID            uint      `gorm:"primaryKey;autoIncrement"`
	ISBN          string    `gorm:"uniqueIndex;not null;size:13"`
	Title         string    `gorm:"not null;size:200"`
	PublishedYear int       `gorm:"not null;check:published_year BETWEEN 1900 AND 2026"`
	Price         float64   `gorm:"type:numeric(10,2);not null;check:price > 0"`
	Stock         int       `gorm:"not null;default:0;check:stock >= 0"`
	CreatedAt     time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
	// TODO: เพิ่มความสัมพันธ์ One-to-Many ไปยัง BorrowRecord (ถ้าต้องการ)
}

// BorrowRecord แทนรายการยืม-คืนหนังสือ (ตาราง borrow_records)
type BorrowRecord struct {
	gorm.Model
	// TODO: กำหนดฟิลด์และ GORM tags ตามข้อกำหนดในโจทย์
	// - MemberID   (uint)       : not null
	// - Member     (Member)     : Belongs To (foreignKey:MemberID, constraint:OnUpdate:CASCADE,OnDelete:RESTRICT)
	// - BookID     (uint)       : not null
	// - Book       (Book)       : Belongs To (foreignKey:BookID, constraint:OnUpdate:CASCADE,OnDelete:RESTRICT)
	// - BorrowDate (time.Time)  : not null, default:CURRENT_TIMESTAMP
	// - DueDate    (time.Time)  : not null
	// - ReturnDate (*time.Time) : nullable
	// - Status     (string)     : not null, default:'BORROWED', check:status IN ('BORROWED','RETURNED','OVERDUE')
}
