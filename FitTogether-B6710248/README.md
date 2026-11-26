# FitTogether

ระบบโพสต์และระบบสร้างกลุ่มในโฟลเดอร์เดียว ใช้ Nuxt 4, Go REST API และ PostgreSQL หน้าฟีดและหน้ากลุ่มใช้ดีไซน์ Sport Editorial พื้นเข้ม ภาพกีฬา และสีส้ม #f0421e

## เริ่มระบบ

ต้องมี Docker Desktop, Go 1.25 ขึ้นไป และ Node.js 20 ขึ้นไป เปิด PowerShell ในโฟลเดอร์นี้ แล้วรัน

```powershell
.\start-dev.ps1
```

เปิด http://127.0.0.1:3000 ตัวเริ่มระบบจะเปิด Docker Desktop เมื่อ Engine ยังไม่ทำงาน จากนั้นเปิด PostgreSQL, รัน migration, เปิด Go API และเปิด Nuxt คำสั่งนี้ใช้ Docker Compose project ชื่อ `fittogether-backend` และ volume `fittogether-backend_data` เดียวกับชุด Backend เดิม จึงเห็นข้อมูลชุดเดิม หาก Go API ที่พอร์ต 8080 ทำงานอยู่แล้ว ตัวเริ่มระบบจะใช้ API นั้น กด Ctrl+C เพื่อหยุด Frontend และ Go API ที่สคริปต์เปิดเอง ฐานข้อมูลยังทำงานต่อ

บัญชีทดลอง Tom, Mint และ Beam อยู่ในแถบข้างบนหน้าจอคอมพิวเตอร์ API ใช้ token development ของบัญชีเหล่านี้

## สำรองข้อมูล

```powershell
.\backup-database.ps1
```

ไฟล์สำรองฐานข้อมูลและรูปโพสต์จะอยู่ใน `backups` ภายในโฟลเดอร์นี้ สำรองก่อนย้ายข้อมูลหรือเปลี่ยน schema

## โครงสร้าง

- `backend` มี Go API, migration และคู่มือ API
- `frontend` มี Nuxt UI และ proxy จาก `/api` ไปที่ Go API
- `compose.yaml` กำหนด PostgreSQL สำหรับการพัฒนา

หน้าสร้างโพสต์เลือกรูป JPEG, PNG หรือ WebP จากเครื่องหรือมือถือได้ และวางรูปด้วย Ctrl+V ได้ สูงสุด 10 รูป ขนาดไม่เกิน 8 MB ต่อรูป รูปเก็บในโฟลเดอร์ `media` ของโปรเจกต์นี้ รูปหน้าปกกลุ่มยังใช้ HTTPS URL เมื่อเชื่อมระบบสมาชิกจริง ให้เปลี่ยนการส่ง token ที่ `frontend/app/composables/useBackend.ts` และตั้งค่า JWT ฝั่ง Go ให้ตรงกับระบบสมาชิกของทีม
