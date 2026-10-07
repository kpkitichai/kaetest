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
| `STORAGE` | `firestore` หรือ `file` (ค่าเริ่มต้น `file`; Docker image ตั้งเป็น `firestore`) |
| `GOOGLE_CLOUD_PROJECT` | Project ID ของ Firestore (บน Cloud Run หาให้เอง) |
| `DATA_FILE` | path ไฟล์ JSON เมื่อ `STORAGE=file` (ค่าเริ่มต้น `data.json`) |
| `CONTACT_EMAIL` | อีเมลที่แสดงในหน้า `/privacy` |
| `DEV_MODE` | `1` = รับ token `dev:<name>` เพื่อรันนอก LINE — **ห้ามเปิดใน production** |
| `PORT` | ค่าเริ่มต้น `8080` |

## Deploy ขึ้น Cloud Run + Firestore

```sh
gcloud config set project YOUR_PROJECT_ID
gcloud services enable run.googleapis.com firestore.googleapis.com cloudbuild.googleapis.com
gcloud firestore databases create --location=asia-southeast1      # ครั้งแรกครั้งเดียว

gcloud run deploy big-catch --source . --region asia-southeast1 --allow-unauthenticated \
  --set-env-vars LINE_CHANNEL_ID=xxxxxxxxxx,LIFF_ID=xxxxxxxxxx-xxxxxxxx,CONTACT_EMAIL=you@example.com
```

service account ของ Cloud Run ต้องมี role `roles/datastore.user` (default compute service account มักมีสิทธิ์อยู่แล้ว)

## ตั้งค่าใน LINE Developers Console

1. LINE MINI App channel → แท็บ **Web app settings**
2. ใส่ **Endpoint URL** = URL ของ Cloud Run (เช่น `https://big-catch-xxxx.a.run.app`)
3. Scopes: เลือก `openid` และ `profile`
4. Privacy policy URL = `https://<cloud-run-url>/privacy`
5. เปิดลิงก์ `https://miniapp.line.me/<LIFF_ID>` ในแอป LINE บนมือถือเพื่อทดสอบ
