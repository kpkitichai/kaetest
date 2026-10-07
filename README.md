# บัญชีปลาใหญ่ (Big Catch Ledger) — LINE MINI App

จด **เฉพาะรายจ่ายพิเศษ** ที่เพิ่มขึ้นมาจากปกติในแต่ละเดือน เช่น ทำฟัน ซ่อมแอร์ ซองงานแต่ง
รายจ่ายเล็กๆ ไม่ต้องจด ("ปลาตัวเล็ก ปล่อยคืนทะเล") แต่ละรายการเป็นจานซูชิ สีจานบอกขนาดเทียบกับค่าใช้จ่ายปกติ:
ปลาทู < 5%, แซลมอน < 15%, ทูน่า < 30%, ฉลามยักษ์ ≥ 30%

ทำงานเป็น LINE MINI App (LIFF): ล็อกอินด้วย LINE อัตโนมัติ ข้อมูลแยกตาม LINE user ID

## รันบนเครื่อง (ไม่ต้องใช้ LINE)

```sh
DEV_MODE=1 go run .          # http://localhost:8080
                             # http://localhost:8080/?user=bob  จำลองผู้ใช้อีกคน
go test ./...
```

## Environment variables

| ชื่อ | ค่า |
|---|---|
| `LINE_CHANNEL_ID` | Channel ID ของ LINE MINI App channel (ใช้ตรวจ ID token) — **จำเป็น** ใน production |
| `LIFF_ID` | LIFF ID (Developing / Review / Published ใช้คนละค่า) |
| `DATABASE_URL` | Postgres connection string (Supabase) — ถ้าไม่ใส่จะเก็บลงไฟล์ JSON |
| `DATA_FILE` | path ไฟล์ JSON เมื่อไม่มี `DATABASE_URL` (ค่าเริ่มต้น `data.json`) |
| `CONTACT_EMAIL` | อีเมลที่แสดงในหน้า `/privacy` |
| `DEV_MODE` | `1` = รับ token `dev:<name>` เพื่อรันนอก LINE — **ห้ามเปิดใน production** |
| `PORT` | ค่าเริ่มต้น `8080` |

เทสกับ Postgres: `TEST_DATABASE_URL=postgres://... go test ./...` (ตารางจะถูกล้าง — ใช้ฐานข้อมูลทดสอบเท่านั้น)

## 1. Supabase (ฐานข้อมูล)

1. สร้าง project ที่ [supabase.com](https://supabase.com) — region **Southeast Asia (Singapore)**
2. กด **Connect** → เลือก **Session pooler** → คัดลอก connection string (รองรับ IPv4) แล้วแทน `[YOUR-PASSWORD]`
3. ไม่ต้องสร้างตารางเอง แอปจะรัน `schema.sql` ให้ตอนเริ่ม (เปิด RLS ไว้ เพื่อไม่ให้ใครอ่านข้อมูลผ่าน Supabase REST API ได้)

## 2. Hostinger VPS (ต้องเป็นแผน VPS — Web/Cloud hosting รัน Go ไม่ได้)

1. ซื้อ VPS (KVM 1 ก็พอ) เลือก OS template **Ubuntu 24.04 with Docker**
2. ตั้ง DNS: เพิ่ม **A record** ของโดเมน/ซับโดเมน (เช่น `bigcatch.yourdomain.com`) ชี้ไปที่ IP ของ VPS
3. SSH เข้า VPS แล้ว:

```sh
git clone https://github.com/kpkitichai/kaetest.git bigcatch && cd bigcatch
git checkout claude/go-hello-world-gffbyq
cp .env.example .env && nano .env      # ใส่ DOMAIN, LINE_CHANNEL_ID, LIFF_ID, DATABASE_URL, CONTACT_EMAIL
docker compose up -d --build
```

Caddy จะขอใบรับรอง HTTPS ให้อัตโนมัติ เปิด `https://<DOMAIN>/privacy` เพื่อเช็กว่าใช้ได้
อัปเดตเวอร์ชันใหม่: `git pull && docker compose up -d --build`

## 3. ตั้งค่าใน LINE Developers Console

1. LINE MINI App channel → แท็บ **Web app settings**
2. **Endpoint URL** = `https://<DOMAIN>`
3. Scopes: เลือก `openid` และ `profile`
4. Privacy policy URL = `https://<DOMAIN>/privacy`
5. เปิดลิงก์ `https://miniapp.line.me/<LIFF_ID>` ในแอป LINE บนมือถือเพื่อทดสอบ
