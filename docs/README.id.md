# procreepy

Utilitas Unix kecil multiplatform (Linux, Windows, macOS): mengekstrak
timelapse arsip yang sudah jadi dari berkas `.procreate` dan menyatukan
segmennya menjadi satu MP4. Tidak ada yang di-encode ulang (stream copy),
tidak ada rendering.

`.procreate` adalah sebuah ZIP. Jika perekaman timelapse diaktifkan, di
dalammnya ada

```text
video/segments/segment-1.mp4
video/segments/segment-2.mp4
...
```

Utilitas mengambil tepat berkas-berkas ini: mengurutkannya secara **numerik**
(`segment-9` sebelum `segment-10`), menganalisis struktur MP4 setiap segmen,
lalu menyusunnya kembali menjadi MP4 moov-first dengan menyalin frame apa ada.
Ia tidak pernah membuka `Document.archive`, layer, atau chunk raster
(`*.lz4`).

## Kebutuhan

Tanpa dependensi eksternal: tidak perlu `ffmpeg`, tidak perlu `ffprobe`.
Untuk kompilasi hanya dibutuhkan Go (versi ada di `go.mod`).

```bash
go build -o procreepy ./cmd/procreepy
```

### Membangun untuk sistem operasi lain

Proyek ini murni Go dan dapat di-cross-compile dengan bersih untuk semua
target yang didukung. Dari platform apa pun, salah satu perintah ini
berfungsi:

| Target | Perintah |
|---|---|
| Linux x86-64 | `GOOS=linux GOARCH=amd64 go build -o procreepy ./cmd/procreepy` |
| Linux ARM 64-bit (Raspberry Pi, Graviton) | `GOOS=linux GOARCH=arm64 go build -o procreepy ./cmd/procreepy` |
| Linux ARM 32-bit | `GOOS=linux GOARCH=arm GOARM=7 go build -o procreepy ./cmd/procreepy` |
| Windows x86-64 (10/11) | `GOOS=windows GOARCH=amd64 go build -o procreepy.exe ./cmd/procreepy` |
| Windows ARM 64-bit | `GOOS=windows GOARCH=arm64 go build -o procreepy.exe ./cmd/procreepy` |
| macOS Intel | `GOOS=darwin GOARCH=amd64 go build -o procreepy ./cmd/procreepy` |
| macOS Apple Silicon (M1–M5) | `GOOS=darwin GOARCH=arm64 go build -o procreepy ./cmd/procreepy` |

Semua build bersifat statis (tanpa cgo): biner Linux berjalan di distribusi
apa pun, berapapun versi glibc-nya. Pipeline GitLab CI membangun tepat
target-target ini di setiap commit; job `dist` menerbitkan tarball beserta
manifest `SHA256SUMS`, dan job `repro:*` membuktikan bahwa biner tersebut
dapat direproduksi bit demi bit.

- Linux/macOS: tidak ada langkah instalasi, jalankan binernya langsung.
- Windows: biner ini tidak bertanda tangan, jadi SmartScreen mungkin
  menampilkan "PC Anda telah dilindungi" — pilih **Info lainnya → Jalankan
  saja**.

## Pemakaian

Satu berkas:

```bash
procreepy artwork.procreate artwork.mp4
procreepy artwork.procreate > artwork.mp4
cat artwork.procreate | procreepy - > artwork.mp4
procreepy --list artwork.procreate
procreepy --verify artwork.procreate
procreepy --split artwork.procreate artwork.mp4
```

Empat kombinasi `INPUT`/`OUTPUT` didukung: `FILE OUTPUT`, `FILE -`,
`- OUTPUT`, `- -`. Jika `OUTPUT` dilewati, tujuannya stdout. Jika `OUTPUT`
adalah direktori yang sudah ada, videonya diletakkan di sana dengan nama
aslinya (`procreepy art.procreate videos/` → `videos/art.mp4`).

### Mode batch: satu folder berisi `.procreate` → satu folder berisi video

Skenario "di `input/` penuh `.procreate`, dan saya ingin videonya di
`output/timelaps/`":

```bash
procreepy input/
```

```text
input/                             output/timelaps/
├── Portrait of a Cat.procreate →  ├── Portrait of a Cat.mp4
├── Landscape v2.procreate      →  ├── Landscape v2.mp4
└── No Timelapse.procreate            └── (dilewati, dengan peringatan)
```

- **Nama**: `<nama asli tanpa .procreate>.mp4`. Spasi, huruf Kiril, dan
  karakter khusus dipertahankan apa adanya.
- **Direktori keluaran**: bawaan `output/timelaps/` relatif terhadap direktori
  saat ini, dibuat otomatis. Bisa diberi yang lain sebagai argumen kedua:
  `procreepy input/ ~/Videos/procreate`.
- **Menjalankan ulang aman**: video yang sudah ada dilewati. Untuk membangun
  ulang semuanya: `--force` (`-f`).
- **`-r`** juga masuk ke subfolder; struktur subfolder dicerminkan pada hasil
  (`input/2025/Cat.procreate` → `output/timelaps/2025/Cat.mp4`), sehingga nama
  sama di folder berbeda tidak saling bertabrakan.
- **Satu berkas rusak tidak menghentikan yang lain.** Berkas tanpa timelapse
  (perekaman dimatikan) adalah peringatan, bukan kesalahan. Berkas korup adalah
  kesalahan: ia muncul dalam ringkasan akhir, dan kode keluar menjadi `1`.
- Berkas tersembunyi (`._Foo.procreate`, ditinggalkan macOS saat menyalin)
  diabaikan.
- Berkas asli tidak pernah diubah.

Contoh keluaran (semuanya ke stderr):

```text
info: 4 .procreate file(s) in input -> output/timelaps/
info: [1/4] input/Landscape v2.procreate -> output/timelaps/Landscape v2.mp4
warning: [2/4] input/No Timelapse.procreate: no timelapse video inside, skipped
error: [3/4] input/Corrupt file.procreate: input is not a valid ZIP archive ...
info: [4/4] input/Portrait of a Cat.procreate -> output/timelaps/Portrait of a Cat.mp4
info: summary: 2 converted, 1 without timelapse, 1 FAILED
```

`--list` dan `--verify` juga menerima direktori dan menjelajahi semua
berkasnya.

### Pemisahan: video + proyek ringan (`--split`)

Idenya: timelapse memakan ruang lebih banyak daripada gambar itu sendiri —
misalnya saat backup ke iPad, mungkin ingin dijaga terpisah. `--split`
menulis, di samping setiap `MP4` yang selesai, salinan proyek yang **tanpa**
segala isi di bawah `video/`:

```bash
procreepy --split artwork.procreate artwork.mp4
```

```text
artwork.procreate  →  artwork.mp4                    (timelapse-nya, lossless)
                     →  artwork.procreepy.procreate  (proyek yang sama, tanpa video/)
```

Dalam mode batch, hal yang sama: di samping setiap `X.mp4` muncul
`X.procreepy.procreate`. Semua anggota arsip lainnya (layer, `Info.plist`,
preview) dibawa byte demi byte: urutan, metode kompresi, dan timestamp
dipertahankan. `.procreate` asli tidak diubah; salinan ringan tidak bisa
ditulis ke stdout, jadi `--split` menuntut `OUTPUT` berupa berkas.

### Diagnostik

```bash
procreepy --list artwork.procreate
```

```text
input: artwork.procreate
segments: 12

1  video/segments/segment-1.mp4
2  video/segments/segment-2.mp4
...
12 video/segments/segment-12.mp4
```

```bash
procreepy --verify artwork.procreate
```

Menganalisis setiap segmen langsung dari arsip (termasuk pengecekan CRC
di dalam ZIP), mencetak laporan baris demi baris, dan memastikan segmen-segmen
bisa disambungkan tanpa encode ulang. **Tidak ada video keluaran yang
dibuat.** `--list` hanya membaca daftar isi ZIP.

## Opsi

| Opsi | Fungsi |
|---|---|
| `-r`, `--recursive` | masukan direktori: jelajahi juga subfolder |
| `-f`, `--force` | masukan direktori: timpa video yang sudah ada |
| `--strict` | perlakukan nomor segmen yang hilang sebagai kesalahan (bawaannya peringatan) |
| `--reencode` | diterima demi kompatibilitas skrip lama; tidak ada encode ulang, selalu stream copy |
| `--split` | tulis `X.procreepy.procreate` di samping tiap `MP4` — proyek tanpa `video/` |
| `--tmpdir DIR` | ke mana segmen diekstrak |
| `-q`, `--quiet` | hanya cetak peringatan dan kesalahan |

## Cara kerja

1. `INPUT` adalah berkas atau stdin. Stdin (dan masukan apa pun yang tidak
   dapat ditelusuri) dulu dibuang ke berkas sementara, karena ZIP membutuhkan
   akses acak.
2. ZIP divalidasi (hanya baca), lalu entri `video/segments/segment-N.mp4`
   dicari.
3. Pengurutan numerik. Lompatan penomoran adalah peringatan; nama tanpa nomor
   diabaikan dengan peringatan.
4. Setiap segmen dianalisis langsung dari ZIP (tanpa ekstraksi penuh): box
   MP4, ukuran track, parameter codec. Kerusakan pertama — berhenti.
5. Pengecekan kompatibilitas (resolusi, codec, himpunan SPS/PPS, audio).
   Kalau tidak, `-c copy` akan diam-diam menghasilkan sampah — sehingga
   ketidakcocokan adalah kesalahan dengan pesan jelas, bukan kejutan di video
   jadi.
6. MP4 moov-first dirakit: `ftyp`, `moov` (semua track, dipotong dari
   segmen-segmen), lalu `mdat` menyusul `mdat` sesuai urutan pemutaran.
7. Berkas sementara (jika ada) selalu dihapus — saat sukses, saat gagal, saat
   Ctrl+C, dan saat SIGTERM.

### Menulis ke berkas dan ke stdout

Kedua jalur merakit MP4 moov-first yang sama: atom moov ditulis duluan karena
frame disalin langsung dari segmen sumber dan metadatanya sudah diketahui
sebelum menulis dimulai. Untuk berkas, ini MP4 "klasik", berguna baik untuk
player maupun editor; berkas persis yang sama ikut masuk ke pipa —
`> artwork.mp4` memberi hasil yang sama dengan
`procreepy artwork.procreate artwork.mp4` eksplisit.

- Keluaran ke **berkas** bersifat atomik: berkas `.partial` di samping
  tujuan, diganti namanya hanya setelah berhasil. Jalanan yang gagal tidak
  meninggalkan sisa dan tidak pernah merusak berkas yang sudah ada.
- stdout tidak pernah ternoda teks. Semua baris `info:`/`warning:`/`error:`
  pergi ke stderr. Satu-satunya pengecualian adalah laporan
  `--list`/`--verify`, di mana stdout *adalah* hasilnya. Jika stdout adalah
  terminal, utilitas menolak membongkar MP4 biner ke sana.

### Berkas sementara dan Fedora

Di Fedora, `/tmp` adalah tmpfs di RAM. Segmen timelapse bisa ratusan
megabita, dan saat membaca dari stdin seluruh `.procreate` dibuang ke sana.
Jadi direktori sementara dipilih begini: `--tmpdir` → `$TMPDIR` →
`/var/tmp` (di disk). Ruang kosong dicek sebelum ekstraksi; jika kurang,
yang keluar kesalahan jelas dengan petunjuk, bukan "No space left" di tengah
pekerjaan.

## Kode keluar

| Kode | Arti |
|---|---|
| 0 | sukses |
| 1 | kesalahan tak terduga; dalam mode batch — minimal satu berkas gagal |
| 2 | argumen salah; keluaran akan menimpa masukan; stdout adalah terminal |
| 3 | masukan tidak ditemukan, kosong, atau bukan ZIP |
| 4 | tidak ada `video/segments` di arsip (timelapse tidak direkam) |
| 5 | segmen korup; penomoran ambigu atau hilang (`--strict`) |
| 6 | dicadangkan (tidak dipakai: tanpa dependensi eksternal) |
| 7 | segmen-segmen tidak cocok untuk stream copy |
| 8 | dicadangkan (tidak dipakai: tanpa dependensi eksternal) |
| 9 | kegagalan menulis hasil atau berkas sementara |
| 130 | interupsi (Ctrl+C / SIGTERM) |

## Uji

```bash
go test ./...
```

Tidak butuh `.procreate` sungguhan: uji membangun ZIP dari segmen MP4 yang
dihasilkan (lihat `internal/testkit`). Pengecekkan struktural: analisis MP4
hasil, urutan box, jumlah sampel, isi `mdat`. `-race` tidak wajib, tapi bekerja
jika ada compiler C yang terpasang.

Tertutup: berkas biasa, ketiadaan `video/segments`, satu segmen, segmen
acau urutannya (`segment-9`/`segment-10`), stdin, stdout, spasi dan karakter
khusus dalam nama, ZIP rusak dan terpotong, MP4 rusak dan terpotong, kerusakan
CRC, kesalahan menulis (`/dev/full`), segmen tak cocok, serta seluruh mode
batch.

## Apa yang sengaja tidak dilakukan

Tidak menganalisis `Document.archive` (NSKeyedArchive), tidak menyentuh
`*.lz4`, tidak memulihkan layer, dan tidak merender gambar. Jika timelapse
tidak pernah direkam di berkas, utilitas ini tidak bisa mengambilnya kembali
dari riwayat menggambar. Catatan: `lz4 -t` pada `.lz4` yang dikeluarkan dari
`.procreate` bukan pengecekan integritas — mereka bukan frame LZ4 mandiri.

## Rujukan format

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer

## Lisensi

Apache License 2.0, lihat `LICENSE`.

---

## Bahasa

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
