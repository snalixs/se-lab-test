# Midterm Lab Exam - Starter Code

โปรเจกต์เริ่มต้นสำหรับฝึกซ้อมสอบปฏิบัติการกลางภาค (อ้างอิงเนื้อหา Lab 03: PostgreSQL Constraints + GORM & Go)

## โครงสร้างโปรเจกต์
- `docker-compose.yml` — สำหรับรัน PostgreSQL 16 บน Docker
- `config/database.go` — ตั้งค่าการเชื่อมต่อ PostgreSQL ด้วย GORM
- `sql/01_schema.sql` — ไฟล์สำหรับเขียนคำสั่ง SQL ในข้อที่ 1
- `models/models.go` — ไฟล์สำหรับเขียน GORM Structs และ Relationships ในข้อที่ 2
- `main.go` — ไฟล์หลักสำหรับทดสอบเชื่อมต่อและ AutoMigrate
- `tests/constraints_test.go` — ไฟล์สำหรับเขียน Unit Tests ตรวจสอบ Constraints ในข้อที่ 3

## วิธีเริ่มต้นใช้งาน
1. เปิด PostgreSQL ด้วย Docker:
   ```powershell
   docker compose up -d
   ```
2. ดาวน์โหลด Go dependencies:
   ```powershell
   go mod tidy
   ```
3. ทดสอบรันโปรแกรมหลัก:
   ```powershell
   go run .
   ```
4. รัน Unit Tests:
   ```powershell
   go test -v ./tests/...
   ```
5. เข้าใช้งาน `psql` ใน Docker Container:
   ```powershell
   docker exec -it midterm_exam_db psql -U postgres -d library_db
   ```
