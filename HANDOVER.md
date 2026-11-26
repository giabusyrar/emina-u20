# Memindahkan Explorer Quiz (Emina 20 Under 20)

Panduan untuk memindahkan proyek ini ke mesin, folder, atau orang lain.
Untuk cara kerja konten dan scoring, baca [README.md](README.md) — dokumen ini
hanya soal **pemindahan**.

- **Live (production):** https://emina-20-under-20.vercel.app
- **Ukuran folder:** 15 MB (`art/` 14 MB di antaranya)
- **Stack:** satu file `index.html` statis. Tanpa framework, tanpa dependency
  runtime, tanpa build step selain satu skrip Python.

---

## 1. Baca ini dulu: proyeknya bukan repo sendiri

Folder `story-game/` ini **menumpang di dalam repo lain**. Repo-nya adalah
layanan Go `seagull-core-engine`:

```
emina-u20/                     <- git root (repo Go, remote GitLab di bawah)
├── go.mod  cmd/  internal/    <- seagull-core-engine, JANGAN disentuh
├── docs/  AGENTS.md
└── story-game/                <- quiz ini, satu-satunya yang relevan
```

Remote: `https://gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/emina-u20.git`

Artinya:

- `git clone` repo itu **ikut membawa seluruh layanan Go**. Kalau yang dibutuhkan
  cuma quiz-nya, salin foldernya saja (lihat §3).
- Jangan jalankan `git add .` dari git root — itu akan mencampur file Go.
- Semua perintah di dokumen ini dijalankan **dari dalam `story-game/`**.

## 2. Ada pekerjaan yang belum di-commit

Per 26 November 2026, isi working tree **lebih baru daripada commit terakhir**:

| | jumlah |
|---|---|
| File termodifikasi tapi belum di-commit | 15 |
| File baru yang belum di-track sama sekali | 19 |

Commit terakhir yang menyentuh folder ini: `7c5b8a0`.

Yang belum di-track mencakup aset yang **tidak bisa dibuat ulang tanpa file
desain sumber**: seluruh `art/speakers/` (28 foto), keenam plate `art/kv/reveal-*.jpg`,
`art/kv/explorer-summit.png`, `art/kv/cover.jpg`, dan keenam maskot `art/char/t_*.png`.

> **Jangan pindahkan proyek ini lewat `git clone` saja** — hasilnya akan kehilangan
> hampir semua aset final dan versi live yang sekarang. Commit dulu, atau salin
> seluruh folder (§3).

Untuk melihat persisnya:

```bash
git status --porcelain story-game
```

## 3. Cara memindahkan

### Opsi A — salin foldernya (paling aman, direkomendasikan)

Membawa semua aset termasuk yang belum di-commit.

```bash
rsync -a --exclude '.DS_Store' --exclude '__pycache__' \
  "/Users/<user>/Documents/Projects/emina/20-under-20-quiz/story-game/" \
  "<tujuan>/emina-explorer-quiz/"
```

Setelah itu lihat §4 (token) lalu §5 (build & deploy). Tidak ada path absolut di
dalam kode, jadi foldernya bisa diletakkan di mana saja — satu-satunya yang
bergantung pada lokasi adalah pencarian token Vercel (§4).

### Opsi B — commit dulu, baru clone

Pakai ini kalau penerimanya memang butuh riwayat git.

```bash
cd <git root>
git add story-game
git commit -m "story-game: Explorer Quiz assets and rundown layout"
git push          # push dilakukan oleh pemilik repo, bukan otomatis
```

Lalu `git clone` dan ambil `story-game/`. Perlu diingat clone-nya tetap membawa
layanan Go di root.

### Opsi C — repo baru khusus quiz

Kalau mau dilepas dari repo Go: salin seperti Opsi A, lalu `git init` di folder
hasil salinan. Riwayat lama tidak terbawa — kalau riwayat itu penting, pakai
`git filter-repo --subdirectory-filter story-game` pada klon repo lama.

## 4. Token Vercel — tidak ikut terbawa

`deploy.py` membaca token di tempat, tidak pernah menyalin atau mencetaknya. Token
dicari berurutan: `.vercel-token.local` di sebelah skrip dulu, lalu beberapa lokasi
cadangan milik proyek photobooth.

**Di mesin ini token itu berada di luar folder proyek**, yaitu di
`Documents/Projects/foto booth/paragon-photobooth/.vercel-token.local`. Jadi menyalin
`story-game/` saja **tidak** membawa token, dan deploy akan berhenti dengan pesan
"No Vercel token found".

Di mesin tujuan, buat token di https://vercel.com/account/tokens lalu simpan:

```bash
printf '%s' '<TOKEN>' > story-game/.vercel-token.local
```

File itu sudah ter-cover `.gitignore` lewat pola `*.local`, jadi tidak akan ikut
ter-commit. Jangan menaruh token di dalam `deploy.py`.

Target deploy sudah tertulis di `deploy.py` dan tidak perlu diubah:

| | |
|---|---|
| Team ID | `team_Akado7k1R3JZGlH3jxyo8cpg` |
| Project | `emina-20-under-20` |
| Domain | `emina-20-under-20.vercel.app` |

Akun pemegang token harus punya akses ke team tersebut. Kalau dipindah ke akun
Vercel lain, ganti `TEAM_ID` dan `PROJECT` di `deploy.py` dan domainnya akan ikut
berubah.

## 5. Prasyarat & perintah

Build dan deploy **hanya butuh Python 3 dari standard library** — tidak ada
`pip install`, tidak butuh Node maupun Vercel CLI. Diuji dengan Python 3.9.6.

```bash
cd story-game

python3 build/build4.py          # menghasilkan index.html
python3 -m http.server 8777      # preview di http://localhost:8777
python3 deploy.py                # preview deployment
python3 deploy.py --prod         # production
```

Library pihak ketiga hanya dibutuhkan kalau mau **mengekstrak aset dari file
desain** (§7), bukan untuk build atau deploy:

```bash
python3 -m pip install --user psd-tools Pillow PyMuPDF
```

## 6. Mana yang sumber, mana yang hasil

`index.html` **dihasilkan** — jangan diedit langsung, perubahannya akan hilang di
build berikutnya.

| Path | Peran |
|---|---|
| `build/template4.html` | Engine: markup, CSS, JS, generator share card. **Di sinilah layout diedit.** |
| `story/v6-explorer.json` | Chapter, pertanyaan, 6 tipe Explorer, string UI |
| `story/event.json` | Data Summit: tanggal, lokasi, booth, 8 sesi stage + speaker |
| `build/build4.py` | Menyatukan ketiganya menjadi `index.html` |
| `index.html` | **Hasil build.** Ini yang di-deploy |
| `deploy.py` | Upload ke Vercel lewat REST API (upload per SHA-1) |
| `art/` | Gambar |

`build4.py` menyisipkan isi file JSON ke dalam token `/*__STORY__*/`,
`/*__EVENT__*/`, `/*__FB__*/`, `/*__SPRITES__*/`, `/*__SIMPLE__*/`,
`/*__TYPEIMG__*/`, dan `__VER__` di template. Kalau salah satu token hilang dari
template, build berhenti dengan pesan jelas.

`build/` juga berisi `build.py`, `build2.py`, `build3.py`, `template.html` sampai
`template3.html`, dan beberapa skrip generator. Itu semua **versi lama** dari
iterasi sebelumnya. Yang dipakai sekarang **hanya `build4.py` + `template4.html`**.

## 7. Aset: dari mana asalnya

File desain sumber **tidak ada di dalam repo** — semuanya ada di `~/Downloads`
pada mesin ini dan perlu diminta ulang dari tim desain kalau butuh ekstraksi
ulang.

| File sumber | Ukuran | Dipakai untuk |
|---|---|---|
| `Emina20-phase2-3-rundown.psd` | 942 MB | Rundown 8 sesi + 26 foto speaker (`art/speakers/`) |
| `Result Page.ai` | 88 MB | Lockup "EXPLORER SUMMIT" (`art/kv/explorer-summit.png`), layout result page |
| `Emina-20U20-UI QUIZ-2.psd` | 42 MB | 6 background reveal (`art/kv/reveal-*.jpg`) |
| `Emina-20U20-Invitation Rundown.psd` | 965 MB | Rundown versi sebelumnya |
| `PREVIEW MASCOT.psd` | 25 MB | 6 maskot tipe (`art/char/t_*.png`) |
| `Emina 20U20 - Personality Quiz (1).pptx` | 14 MB | Deck konsep + feedback |

Folder Drive rundown speaker:
https://drive.google.com/drive/folders/1-FFdlzV7BI3sLxJ75CeN8XPcvnX4Fy1h

Catatan penting soal foto speaker: di dalam PSD setiap speaker punya **dua layer**
— foto asli dan satu siluet cyan yang di-offset beberapa piksel sebagai efek
outline. Kalau mengekstrak ulang, layer siluet itu harus dibuang (dideteksi lewat
dominasi hue cyan), kalau tidak yang keluar adalah bayangan biru, bukan orangnya.

## 8. File yang ikut ter-upload padahal tidak terpakai

`deploy.py` mengirim `index.html` plus **semua** `.jpg`/`.png`/`.webp` di dalam
`art/` (kecuali nama berawalan `_` dan `sheet.png`). Angkanya sekarang:

- 89 file terkirim ke Vercel
- 53 file benar-benar dirujuk `index.html`

Selisihnya adalah sisa iterasi lama: 14 gambar `art/a*.jpg` + `art/hero.jpg`
(scene cerita versi 1–5) dan sprite maskot `art/char/p00..p15.png` + `poses.json`
(mode sprite lama; build sekarang pakai `art/char/hero.png`).

Ini **tidak merusak apa pun** — hanya menambah berat deploy sekitar 2,5 MB untuk
gambar legacy dan beberapa MB untuk sprite. Kalau mau dibersihkan, pindahkan ke
subfolder berawalan `_` (otomatis dilewati `deploy.py`) dan jangan dihapus, karena
versi cerita lama (`story/v1-story.json` … `v5-story.json`) masih merujuknya.

## 9. Memastikan pemindahannya berhasil

Setelah dipindah, jalankan urutan ini di mesin tujuan:

```bash
cd story-game
python3 build/build4.py
```

Keluaran yang benar:

```
scenes 7  art 7/7  type images 6  sprites yes
wrote .../index.html (xx.x KB)
```

`art 7/7` dan `type images 6` adalah pemeriksaan kelengkapan aset. Kalau angkanya
lebih kecil, ada gambar yang tidak ikut tersalin — jangan di-deploy dulu.

Lalu buka preview dan periksa secara manual, karena build tidak memvalidasi hal
berikut:

1. Selesaikan kuis sampai halaman result.
2. Tab **Explorer card** — maskot tipe muncul, background reveal berwarna.
3. Tab **Summit guide** — lockup "EXPLORER SUMMIT" tampil sebagai gambar (bukan
   teks), dan tiap baris stage memuat foto speaker beserta chip namanya.
4. Tekan **Save / Share** — share card 1080×1920 harus ter-render penuh.

## 10. Hal yang masih terbuka

- **`THE STYLE EXPLORER` tidak punya layer ground di `UI QUIZ-2.psd`.** Akibatnya
  `art/kv/reveal-ST.jpg` ter-render putih, sementara lima tipe lain punya ground
  berwarna. Kemungkinan besar ini kelalaian desainer dan **belum dikonfirmasi ke
  klien**.
- **Tiga nama di sesi Music & Fandom masih TBA** (Aldy Maldini / Teuku Rizky /
  Bastian Steel). Tersimpan di `event.json` sebagai field `tba` terpisah dari
  `speakers`, dan dirender sebagai baris abu-abu tersendiri supaya tidak tertukar
  dengan speaker yang sudah pasti. Begitu fix, pindahkan ke `speakers` dan
  tambahkan fotonya.
- **Venue** tertulis "Taman Peruri, Blok M" di `event.json`, sedangkan
  `Result Page.ai` menulis "Taman Kota Peruri, Jakarta Selatan". Sudah
  dikonfirmasi tetap memakai versi Blok M.
- **Foto speaker beresolusi rendah** (sekitar 100–200 px) karena di PSD memang
  ditempatkan kecil. Cukup untuk ukuran tayang sekarang (sekitar 50 px di web,
  116–155 px di share card), tapi tidak bisa diperbesar lagi.
