# Task Specification: Smart Prompting & Autonomous Worker Control in Telegram (AGY04)

## Objektif
Membuat sistem bot Telegram `@Orcyagy_bot` (AgyRemoteTele) lebih cerdas, memiliki prompting komprehensif untuk eksekusi otonom mandiri oleh agy, serta menambahkan fitur kontrol langsung untuk "menghidupkan" worker yang sedang standby/dormant.

## Kebutuhan Implementasi

### 1. Smart Prompt Wrapping (`internal/bot/router.go`)
Di fungsi `handleAsk` (dan fungsi dispatch lainnya):
Perbarui `wrappedPrompt` agar menginstruksikan Antigravity CLI secara kuat dan otonom:
```
[TELEGRAM AGY REMOTE GATEWAY — AUTONOMOUS OPERATOR DIRECTIVE]
Pengirim: Telegram Operator (Chat ID: %d)
Worker Profil: %s
Working Directory: %s

INSTRUKSI PENGGUNA:
%s

STANDAR OPERASIONAL CERDAS (FULL AUTONOMOUS & SELF-HEALING):
1. OTONOMI PENUH: Eksekusi tools, bash command, dan tindakan teknis secara langsung tanpa perlu meminta izin bolak-balik ke operator.
2. SELF-HEALING & MAINTENANCE: Jika menemukan bug, error sistem, atau kegagalan pada bot remote AgyRemoteTele atau script worker, agy WAJIB langsung mengaudit dan memperbaiki (edit) kode terkait hingga tuntas.
3. STANDAR NOKOMEN: DILARANG menulis komentar dalam kode yang dibuat atau diedit.
4. GIT-FIRST: Pastikan setiap perubahan kode yang teruji di-commit dan siap di-push ke GitHub repository.
5. FORMAT RESPON TELEGRAM: Berikan jawaban akhir yang SINGKAT, PADAT, dan TO-THE-POINT mengenai apa yang telah dikerjakan, status perbaikan, dan hasil verifikasi.
```

### 2. Fitur "Hidupkan Worker" (`/up`, `/start_worker`, `/hidupkan`)
- Tambahkan handler command `/up`, `/start_worker`, `/hidupkan`:
  Menerima argumen nama akun (misal `agy02` atau `semua`).
- Dukung juga parsing pesan teks jika operator mengetik pola seperti `hidupkan agyXX` atau `hidupkan semua`.
- Eksekusi:
  Panggil `python3 /opt/data/home/agy-profiles/fast_login.py refresh <account>` (atau tanpa argumen jika semua).
  Kirim respons konfirmasi HTML dengan format elegan bahwa worker telah dihidupkan beserta sisa waktu dan email aktif.

### 3. Penyempurnaan Tampilan Akun (`handleAccounts` & keyboard)
- Di inline keyboard `handleAccounts`:
  Sediakan tombol `⚡ Hidupkan Semua` (`nav:up_all` atau `nav:refresh`) dan tombol worker individual `⚡ <acc>` yang memicu pengaktifan / refresh.
- Jika status akun bertuliskan `Standby` atau `Siap Aktif`, beri emoji `🟡` dan deskripsi yang jelas bahwa akun siap dihidupkan (bukan kedaluwarsa permanen).

### 4. Zero Comments (NOKOMEN)
- Jangan menulis komentar (`//` atau `/* */`) pada kode Go yang baru maupun yang dimodifikasi.

### 5. Unit Tests & Commit
- Tambahkan atau perbarui unit test di `internal/bot/router_test.go` untuk menguji command `/up`, `/hidupkan`, dan prompt formatting.
- Jalankan:
  `export PATH="/opt/data/home/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.1.linux-amd64/bin:$PATH" && go test ./...`
  Pastikan 100% PASS.
- Commit seluruh perubahan ke branch `work/agy04-smart-worker-control`.
