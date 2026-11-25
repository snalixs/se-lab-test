package main

import (
	"fmt"
	"log"

	"midterm-lab-exam/config"
	"midterm-lab-exam/models"
)

func main() {
	db := config.ConnectDatabase()

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("Failed to get underlying sql.DB: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	fmt.Println("Connected to PostgreSQL successfully!")

	// TODO (ข้อที่ 2): เรียก AutoMigrate สำหรับ models.Member{}, models.Book{}, models.BorrowRecord{}
	_ = models.Member{}
}
