# procreepy

Utilitas kecil lintas platform (Linux, Windows, macOS): mengekstrak timelapse
arsip yang sudah jadi dari berkas `.procreate` dan menggabungkan segmennya
menjadi satu MP4. Tidak ada yang di-encode ulang (stream copy), dan tidak ada
rendering.

`.procreate` adalah arsip ZIP. Jika perekaman timelapse diaktifkan, isinya
adalah

```text
video/segments/segment-1.mp4
video/segments/segment-2.mp4
...
```
Utilitas ini mengambil tepat berkas-berkas tersebut: mengurutkannya secara
**numerik** (`segment-9` sebelum `segment-10`), menganalisis struktur MP4 setiap
segmen, lalu menyusunnya kembali menjadi satu MP4 moov-first dengan frame yang
disalur-salin apa adanya. Utilitas tidak pernah membuka `Document.archive`,
layer, atau chunk raster (`*.lz4`).

## Persyaratan

Tidak ada dependensi eksternal: `ffmpeg` dan `ffprobe` tidak diperlukan. Untuk membangun, hanya dibutuhkan Go (versinya ada di `go.mod`) dan `make` (tersedia di semua platform yang didukung; pada sistem minimal, dapat dipasang melalui package manager).
Makefile adalah entry point build yang kanonis: ia menetapkan lingkungan hermetik yang sama seperti yang digunakan CI (mode modul offline, toolchain lokal, tanpa cgo) dan menemukan toolchain Go secara otomatis.

```bash
make build      # kompilasi semuanya dan hasilkan ./procreepy yang dapat dijalankan
make check      # gofmt + build + vet + seluruh test suite
```

`make build` menyematkan `dev-<commit>` ke binary sehingga `procreepy --version` menunjukkan asal commit-nya (tarball rilis menggunakan tag sebagai gantinya). Tanpa `make`, padanan langsungnya adalah `go build ./... && go build -o procreepy ./cmd/procreepy` (binary tersebut melaporkan `dev`, atau `dev-<commit>` bila dibangun di dalam git checkout).

### Membangun untuk sistem operasi lain

Proyek ini murni Go dan dapat di-cross-compile dengan bersih untuk semua target yang didukung. Dari platform mana pun:

| Target | Perintah |
|---|---|
| Linux x86-64 | `make release GOOS=linux GOARCH=amd64` |
| Linux ARM 64-bit (Raspberry Pi, Graviton) | `make release GOOS=linux GOARCH=arm64` |
| Linux ARM 32-bit | `make release GOOS=linux GOARCH=arm` |
| Windows x86-64 (10/11) | `make release GOOS=windows GOARCH=amd64` |
| Windows ARM 64-bit | `make release GOOS=windows GOARCH=arm64` |
| macOS Intel | `make release GOOS=darwin GOARCH=amd64` |
| macOS Apple Silicon (M1–M5) | `make release GOOS=darwin GOARCH=arm64` |
Setiap target menghasilkan `dist/procreepy-<version>-<os>-<arch>.tar.gz` yang ternormalisasi dan mencetak SHA-256-nya; `make cross` membangun seluruh matriks sekaligus, sedangkan `make repro` membuktikan bahwa build dapat direproduksi bit demi bit. Padanan langsung untuk satu target adalah `GOOS=… GOARCH=… go build -o procreepy[.exe] ./cmd/procreepy`.
Semua build bersifat statis (tanpa cgo): binary Linux berjalan pada distro apa pun terlepas dari versi glibc. Pipeline CI (GitLab dan GitHub) membangun target yang sama pada setiap commit; job `dist` menerbitkan tarball beserta manifest `SHA256SUMS`, dan job `repro:*` membuktikan binary dapat direproduksi bit demi bit.

- Linux/macOS: tidak ada langkah instalasi, jalankan binary secara langsung.
- Windows: binary tidak ditandatangani, jadi SmartScreen mungkin menampilkan “Protected your PC” — pilih **More info → Run anyway**.

## Penggunaan

Satu berkas:

```bash
procreepy artwork.procreate artwork.mp4
procreepy artwork.procreate > artwork.mp4
cat artwork.procreate | procreepy - > artwork.mp4
procreepy --list artwork.procreate
procreepy --verify artwork.procreate
procreepy --split artwork.procreate artwork.mp4
```
Keempat kombinasi `INPUT`/`OUTPUT` didukung: `FILE OUTPUT`, `FILE -`, `- OUTPUT`,
`- -`. Jika `OUTPUT` dihilangkan, tujuannya adalah stdout. Jika `OUTPUT` adalah
direktori yang sudah ada, video ditempatkan di sana dengan nama aslinya
(`procreepy art.procreate videos/` → `videos/art.mp4`).

### Mode batch: folder berisi `.procreate` → folder berisi video

Skenarionya: `input/` penuh dengan berkas `.procreate`, dan video ingin
ditempatkan di `output/timelaps/`:

```bash
procreepy input/
```

```text
input/                             output/timelaps/
├── Portrait of a Cat.procreate →  ├── Portrait of a Cat.mp4
├── Landscape v2.procreate      →  ├── Landscape v2.mp4
└── No Timelapse.procreate            └── (dilewati, dengan peringatan)
```
- **Nama**: `<nama asli tanpa .procreate>.mp4`. Spasi, karakter Sirilik, dan
  karakter khusus dipertahankan apa adanya.
- **Folder keluaran**: secara default `output/timelaps/` relatif terhadap
  direktori saat ini, dan dibuat otomatis. Folder lain dapat diberikan sebagai
  argumen kedua: `procreepy input/ ~/Videos/procreate`.
- **Menjalankan ulang aman**: video yang sudah ada akan dilewati. Untuk membangun
  ulang semuanya: `--force` (`-f`).
- **`-r`** juga menelusuri subfolder; struktur subfolder dicerminkan pada hasil
  (`input/2025/Cat.procreate` → `output/timelaps/2025/Cat.mp4`), sehingga nama
  yang sama di folder berbeda tidak bertabrakan.
- **Satu berkas yang rusak tidak menghentikan yang lain.** Berkas tanpa timelapse
  (perekaman dimatikan) adalah peringatan, bukan kesalahan. Berkas korup adalah
  kesalahan: muncul dalam ringkasan akhir, dan kode keluar menjadi `1`.
- Berkas tersembunyi (`._Foo.procreate`, yang ditinggalkan macOS saat menyalin)
  diabaikan.
- Berkas asli tidak pernah diubah.
Contoh keluaran (semuanya ke stderr):

```text
level=INFO msg="batch conversion started" files=4 input=input output=output/timelaps/
level=ERROR msg="file conversion failed" input="input/Corrupt file.procreate" err="input is not a valid ZIP archive: input/Corrupt file.procreate (not a .procreate file, or truncated/corrupted)"
level=INFO msg=converted input="input/Landscape v2.procreate" output="output/timelaps/Landscape v2.mp4"
level=WARN msg="no timelapse video inside, skipped" input="input/No Timelapse.procreate"
level=INFO msg=converted input="input/Portrait of a Cat.procreate" output="output/timelaps/Portrait of a Cat.mp4"
level=INFO msg="batch completed" converted=2 existed=0 no_video=1 failed=1
```
`--list` dan `--verify` juga menerima direktori dan menelusuri semua berkas di dalamnya.

### Pemisahan: video + proyek ramping (`--split`)

Idenya: timelapse memakan lebih banyak ruang daripada gambar itu sendiri —
misalnya, saat membuat cadangan ke iPad, mungkin berguna untuk menyimpannya
terpisah. `--split` menulis, di samping setiap `MP4` yang selesai, salinan proyek
ramping **tanpa** apa pun di bawah `video/`:

```bash
procreepy --split artwork.procreate artwork.mp4
```

```text
artwork.procreate  →  artwork.mp4                    (timelapse, lossless)
                     →  artwork.procreepy.procreate  (proyek yang sama, tanpa video/)
```
Dalam mode batch, sama: di samping setiap `X.mp4` akan muncul
`X.procreepy.procreate`. Semua anggota arsip lainnya (layer, `Info.plist`,
pratinjau) dibawa byte demi byte: urutan, metode kompresi, dan timestamp
dipertahankan. `.procreate` asli tidak diubah; salinan ramping tidak dapat
dituliskan ke stdout, jadi `--split` memerlukan `OUTPUT` berupa berkas.

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
Setiap segmen dianalisis langsung dari arsip (termasuk pemeriksaan CRC di dalam
ZIP), laporan dicetak baris demi baris, dan diperiksa apakah segmen dapat
digabungkan tanpa encode ulang. **Tidak ada video keluaran yang dibuat.**
`--list` hanya membaca direktori ZIP.

## Opsi

| Opsi | Fungsi |
|---|---|
| `-r`, `--recursive` | input direktori: telusuri juga subfolder |
| `-f`, `--force` | input direktori: timpa video yang sudah ada |
| `--strict` | perlakukan nomor segmen yang hilang sebagai kesalahan (default: peringatan) |
| `--reencode` | diterima untuk kompatibilitas dengan skrip lama; tidak ada encode ulang, selalu stream copy |
| `--split` | tulis `X.procreepy.procreate` di samping setiap `MP4` — proyek tanpa `video/` |
| `--tmpdir DIR` | tempat menyimpan file sementara |
| `-q`, `--quiet` | hanya cetak peringatan dan kesalahan |

## Cara kerja

1. `INPUT` adalah berkas atau stdin. Stdin (dan input apa pun yang tidak dapat
   di-seek) terlebih dahulu disimpan ke berkas sementara karena ZIP memerlukan
   akses acak.
2. ZIP divalidasi (hanya baca), lalu entri `video/segments/segment-N.mp4`
   ditemukan.
3. Pengurutan numerik. Celah nomor adalah peringatan; nama tanpa nomor diabaikan
   dengan peringatan.
4. Setiap segmen dianalisis langsung dari ZIP (tanpa ekstraksi penuh): kotak MP4,
   ukuran track, dan parameter codec. Saat korupsi pertama ditemukan, proses berhenti.
5. Pemeriksaan kompatibilitas (resolusi, codec, set SPS/PPS, audio). Jika tidak,
   `-c copy` dapat menghasilkan data yang rusak tanpa pesan — karena itu
   ketidakcocokan menjadi kesalahan dengan pesan yang jelas, bukan kejutan di video akhir.
6. MP4 moov-first dirakit: `ftyp`, `moov` (semua track, diambil dari segmen), lalu
   `mdat` demi `mdat` sesuai urutan pemutaran.
7. Berkas sementara (jika ada) selalu dihapus — saat berhasil, gagal, Ctrl+C,
   maupun SIGTERM.

### Menulis ke berkas dan ke stdout

Kedua jalur merakit MP4 moov-first yang sama: atom moov ditulis lebih dulu karena
frame disalin langsung dari segmen sumber dan metadata sudah diketahui sebelum
penulisan dimulai. Untuk berkas, ini adalah MP4 "klasik" yang cocok untuk player
dan editor; berkas yang sama persis juga masuk ke pipe — `> artwork.mp4` memberi
hasil yang sama dengan `procreepy artwork.procreate artwork.mp4` secara eksplisit.
  * **Output ke berkas** bersifat atomik: `.partial` dibuat di samping target dan
    baru di-rename setelah berhasil. Eksekusi yang gagal tidak meninggalkan sisa
    dan tidak pernah merusak berkas yang sudah ada.
  * stdout tidak pernah tercampur dengan teks. Semua baris log
    (`level=INFO`/`WARN`/`ERROR`, satu record terstruktur key=value per baris)
    dikirim ke stderr. Satu-satunya pengecualian adalah laporan
    `--list`/`--verify`, di mana stdout adalah hasilnya. Jika stdout adalah terminal,
    utilitas menolak menuliskan MP4 biner ke sana.

### Berkas sementara dan Fedora

Di Fedora, `/tmp` adalah tmpfs di RAM. Segmen timelapse dapat berukuran ratusan
megabyte, dan saat membaca dari stdin seluruh `.procreate` disimpan sementara.
Karena itu direktori sementara dipilih dengan urutan: `--tmpdir` → `$TMPDIR` →
`/var/tmp` (di disk) → direktori sistem. Jika disk penuh saat penyimpanan, Anda
mendapat kesalahan yang jelas dengan petunjuk (gunakan `--tmpdir` pada direktori
berbasis disk yang lebih besar), bukan sekadar "No space left".

## Kode keluar

| Kode | Arti |
|---|---|
| 0 | sukses |
| 1 | kesalahan tak terduga; dalam mode batch — setidaknya satu berkas gagal |
| 2 | argumen salah; keluaran akan menimpa masukan; stdout adalah terminal |
| 3 | masukan tidak ditemukan, kosong, atau bukan ZIP |
| 4 | tidak ada `video/segments` di arsip (timelapse tidak direkam) |
| 5 | segmen korup; penomoran ambigu atau hilang (`--strict`) |
| 6 | dicadangkan (tidak digunakan: tanpa dependensi eksternal) |
| 7 | segmen tidak kompatibel untuk stream copy |
| 8 | dicadangkan (tidak digunakan: tanpa dependensi eksternal) |
| 9 | gagal menulis hasil atau berkas sementara |
| 130 | terinterupsi (Ctrl+C / SIGTERM) |

## Pengujian

```bash
make test          # or: go test ./...
```
Berkas `.procreate` nyata tidak diperlukan: pengujian membuat ZIP dari segmen
MP4 yang dihasilkan (lihat `internal/testkit`). Pemeriksaannya bersifat
struktural: parsing MP4 hasil, urutan box, jumlah sample, dan isi `mdat`.
`-race` tidak wajib, tetapi bekerja jika compiler C tersedia.
Yang dicakup: berkas biasa, tidak adanya `video/segments`, satu segmen, segmen
yang urutannya terbalik (`segment-9`/`segment-10`), stdin, stdout, spasi dan
karakter khusus dalam nama, ZIP rusak dan terpotong, MP4 rusak dan terpotong,
kerusakan CRC, kesalahan penulisan (`/dev/full`), segmen tidak kompatibel, dan
seluruh mode batch.

## Yang sengaja tidak dilakukan utilitas ini

Utilitas tidak mengurai `Document.archive` (NSKeyedArchive), tidak menyentuh
`*.lz4`, tidak memulihkan layer, dan tidak merender gambar. Jika timelapse tidak
direkam dalam berkas, utilitas ini tidak dapat memulihkannya dari riwayat gambar.
Catatan: `lz4 -t` pada `.lz4` yang diambil dari `.procreate` bukan pemeriksaan
integritas — itu bukan frame LZ4 mandiri.

## Referensi format

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer

## Lisensi

Apache License 2.0, lihat `LICENSE`.

---

## Bahasa

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
