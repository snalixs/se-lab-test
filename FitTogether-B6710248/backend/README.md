# FitTogether Backend

ระบบโพสต์: ผู้เขียนแก้ไข/ลบคอมเมนต์ได้ และลบโพสต์โดยสำรองข้อมูลใน `deleted_posts` ก่อนนำออกจากตารางใช้งาน ดู [POSTS.md](POSTS.md) และรัน migration 003–004 ก่อนใช้งาน

รายละเอียดระบบกลุ่มแบบหนึ่งกลุ่มต่อหนึ่งกิจกรรม วิธีอัปเกรดข้อมูลเดิม และ API ใหม่ ดู [GROUPS.md](GROUPS.md) ต้องรัน `go run . -migrate` ก่อนเปิด Backend รุ่นนี้ Frontend เดิมยังต้องเพิ่มช่องวันเริ่ม วันจบ และสถานที่ และเปลี่ยนจากกิจกรรมย่อยเป็นกิจกรรมระดับกลุ่ม

## รันด้วยไฟล์ .env

เมื่อเปิดฐานข้อมูลด้วย `docker compose up -d --wait db` จากโฟลเดอร์หลักแล้ว ให้เข้า `backend` และรัน `go run . -migrate` ตามด้วย `go run .` โปรแกรมจะโหลด `backend/.env` อัตโนมัติ หากยังไม่มีไฟล์ให้คัดลอกจาก `.env.example` ค่าตัวอย่างตรงกับ Compose ปัจจุบัน: ฐานข้อมูล `starter` ที่พอร์ต `5432` และ `MEDIA_DIR=../media` ค่าที่ตั้งไว้ใน environment ของ Terminal จะมีลำดับความสำคัญสูงกว่า `.env`

Go 1.25+ REST API สำหรับระบบโพสต์และระบบสร้างกลุ่ม ใช้ PostgreSQL และมีข้อมูลสมาชิกจำลองสำหรับทดลองบนเครื่อง

## ทดลองบน Windows

เปิด PowerShell ในโฟลเดอร์โปรเจกต์และเปิด Docker Desktop ก่อน คำสั่งนี้สร้างฐานข้อมูลสำหรับการพัฒนาที่พอร์ต 55432

```powershell
docker compose up -d --wait db
$env:DATABASE_URL = 'postgres://fittogether:fittogether_dev@127.0.0.1:55432/fittogether?sslmode=disable'
$env:APP_ENV = 'development'
go run . -migrate
go run .
```

ใช้ PostgreSQL ที่มีอยู่แล้วได้โดยเปลี่ยน `DATABASE_URL` ให้ชี้ไปยังฐานข้อมูลทดลอง โหมด development รับคำขอจาก loopback และมีบัญชีจำลอง `dev-tom` (ID 1), `dev-mint` (ID 2) และ `dev-beam` (ID 3) 

เปิด PowerShell อีกหน้าต่างเพื่อทดลองสร้างและอ่านโพสต์

```powershell
$base = 'http://127.0.0.1:8080'
$tom = @{ Authorization = 'Bearer dev-tom' }
Invoke-RestMethod "$base/healthz"
$post = Invoke-RestMethod "$base/api/posts" -Method Post -Headers $tom -ContentType 'application/json; charset=utf-8' -Body (@{
    content = 'วิ่งเช้าที่สนาม'
    visibility = 'public'
    hashtags = @('running')
} | ConvertTo-Json)
Invoke-RestMethod "$base/api/posts?mode=latest" -Headers $tom
```

ทดลองสร้างกลุ่มเปิดแล้วให้ Mint เข้าร่วม

```powershell
$group = Invoke-RestMethod "$base/api/groups" -Method Post -Headers $tom -ContentType 'application/json; charset=utf-8' -Body (@{
    name = 'SUT Run Club'
    description = 'Meet up for a morning run'
    sport = 'running'
    visibility = 'open'
    max_members = 10
    starts_at = (Get-Date).AddDays(1).ToString('o')
    ends_at = (Get-Date).AddDays(2).ToString('o')
    location = 'SUT Stadium'
} | ConvertTo-Json)
$mint = @{ Authorization = 'Bearer dev-mint' }
Invoke-RestMethod "$base/api/groups/$($group.id)/join" -Method Post -Headers $mint
Invoke-RestMethod "$base/api/groups/$($group.id)/members" -Headers $tom
```

## API ที่เรียกจาก Nuxt ได้

ทุกเส้นทางใต้ `/api` ใช้ `Authorization: Bearer <token>` โดยให้ Nuxt ส่งต่อคำขอผ่าน origin เดียวกันได้ รายการใช้ `limit` และ `offset` ชื่อฟิลด์เป็น snake_case และเวลาใน JSON ใช้ RFC 3339

| Method และ path | การใช้งาน |
|---|---|
| GET /api/posts | ฟีด `mode=latest\|following\|popular\|bookmarked` และค้นหาด้วย `q` (ข้อความหรือแฮชแท็ก) |
| POST /api/posts | สร้างโพสต์ด้วย JSON เดิม หรือส่ง multipart/form-data ที่มี `content`, `visibility`, `hashtags` และไฟล์ `images` สูงสุด 10 รูป |
| GET / PATCH / DELETE /api/posts/{post} | เปิด แก้ไข หรือลบโพสต์ โดยการแก้ไขและลบจำกัดตามเจ้าของโพสต์ |
| GET / POST /api/posts/{post}/comments | อ่านหรือส่งคอมเมนต์ ส่ง `content` และ `parent_comment_id` เมื่อตอบกลับ |
| PATCH / DELETE /api/posts/{post}/comments/{comment} | ผู้เขียนแก้ไข `content` หรือลบคอมเมนต์ของตนเอง คำตอบของคนอื่นยังอยู่ |
| PUT / DELETE /api/posts/{post}/like | กดหรือยกเลิกถูกใจ |
| PUT / DELETE /api/posts/{post}/bookmark | บันทึกหรือยกเลิกบันทึก |
| PUT / DELETE /api/users/{user}/follow | ติดตามหรือเลิกติดตาม |
| POST /api/posts/{post}/share | แชร์โพสต์สาธารณะกลับเข้าฟีด พร้อม `content` สำหรับข้อความประกอบ |
| POST /api/posts/{post}/reports | ส่ง `reason` และ `detail` ต่อไปยังระบบความปลอดภัย |
| GET / POST /api/groups | ค้นหาด้วย `q`, `sport`, `visibility` หรือ `scope=mine` และสร้างกลุ่ม |
| GET / PATCH / DELETE /api/groups/{group} | ดูรายละเอียด แก้ไข หรือยกเลิกกลุ่ม (รอลบ 24 ชั่วโมง) |
| POST /api/groups/{group}/join | กลุ่ม open เข้าทันที กลุ่ม closed ส่งคำขอหรือรับคำเชิญ |
| GET /api/groups/{group}/join-requests | เจ้าของดูคำขอเข้ากลุ่ม |
| PUT /api/groups/{group}/join-requests/{request} | เจ้าของส่ง `status` เป็น approved หรือ rejected |
| POST /api/groups/{group}/invites | เจ้าของกลุ่ม closed เชิญสมาชิกด้วย `user_id` |
| GET /api/group-invites | ดูคำเชิญของตนเอง |
| PUT /api/groups/{group}/invites/{invite} | ผู้ได้รับเชิญส่ง `status` เป็น accepted หรือ declined |
| GET /api/groups/{group}/members | สมาชิกดูรายชื่อสมาชิก |
| DELETE /api/groups/{group}/membership | ออกจากกลุ่ม เจ้าของต้องโอนสิทธิ์ก่อนออก |
| DELETE /api/groups/{group}/members/{user} | เจ้าของนำสมาชิกออก |
| PUT /api/groups/{group}/owner | โอนสิทธิ์ให้สมาชิกในกลุ่มด้วย `user_id` |
| GET / POST /api/groups/{group}/messages | สมาชิกอ่านข้อความหรือส่ง `content` |
| POST /api/groups/{group}/complete | เจ้าของจบกิจกรรมหลังเริ่มแล้ว เรียกซ้ำไม่เลื่อนเวลาลบ |
| เส้นทาง /events และ /rsvp เดิม | ตอบ 410 Gone ใช้กลุ่มเป็นกิจกรรมและสมาชิกเป็นผู้เข้าร่วม |
| GET /api/group-notifications | ดูแจ้งเตือนในแอปของตนเอง |
| PUT /api/group-notifications/{notification}/read | ทำเครื่องหมายว่าอ่านแล้ว |

## การเชื่อมกับระบบของทีม

- ให้ migration ของระบบสมาชิกทำงานก่อน แล้วจึงรัน `go run . -migrate` โดยไม่ตั้ง `APP_ENV=development` บนฐานข้อมูลทีม Schema นี้อ้างถึง `users.id` และอ่าน `profiles(user_id, display_name, photo_url)` โดยคาดว่าหนึ่งผู้ใช้มีโปรไฟล์ได้หนึ่งแถว การยืนยันตัวตนอ่าน `is_email_verified` และ `account_status` จาก `users` บัญชีที่ยืนยันอีเมลแล้วและมีสถานะ active จึงใช้ API ได้
- ตั้ง `AUTH_JWT_SECRET` อย่างน้อย 32 ไบต์, `AUTH_JWT_ISSUER` และ `AUTH_JWT_AUDIENCE` ให้ตรงกับระบบสมาชิก รุ่นนี้รับ JWT แบบ HS256 โดย `sub` เป็นเลข ID ผู้ใช้ในรูปข้อความ และต้องมี `exp` ถ้าทีมใช้ token รูปแบบอื่นต้องปรับส่วนยืนยันตัวตนก่อนเชื่อม
- ตั้ง `REPORTS_API_URL` เป็น HTTPS URL ของ endpoint รับรายงาน ระบบจะส่ง bearer token เดิมพร้อม `target_type`, `target_id`, `reason`, `detail` การจัดคิวรายงาน การบล็อกผู้ใช้ และการตัดสินโดย Admin อยู่ที่ระบบความปลอดภัย เมื่อต้องการซ่อนโพสต์จากฟีด ให้ระบบนั้นอัปเดต `posts.status` เป็น `hidden` ผ่านสัญญาการเชื่อมต่อที่ทีมตกลง
- รูปโพสต์จากเครื่องส่งแบบ multipart/form-data รองรับ JPEG, PNG และ WebP สูงสุด 10 รูป รูปละไม่เกิน 8 MB เก็บใน `MEDIA_DIR` หรือ `media` เมื่อไม่ตั้งค่า และอ่านผ่าน `GET /media/{file}` รูปที่มี URL นี้เปิดได้โดยผู้ที่มีลิงก์ ผู้ใช้ JSON เดิมยังส่ง HTTPS URL ใน `images` ได้ ส่วน `cover_url` ของกลุ่มยังรับ HTTPS URL
- ใน ER Diagram ใช้ `int`, `string`, `float`, `boolean` ตามรูปแบบเอกสารของทีม ส่วน migration ใช้ชนิดข้อมูล PostgreSQL เช่น `bigint` สำหรับ ID และ `timestamptz` สำหรับเวลา
