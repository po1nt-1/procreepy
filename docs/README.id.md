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
disalur-salin apa adanya. Dalam mode batch, ia juga menulis salinan proyek
yang bisa diimpor kembali tanpa timelapse untuk setiap karya yang
dikonversi, serta — dengan `--psd` — sebuah PSD Photoshop berlayer. Jalur
timelapse tidak pernah membuka `Document.archive`, layer, atau chunk raster
(`*.lz4`); hanya `--psd` yang melakukannya.

## Persyaratan

Tidak ada dependensi eksternal: `ffmpeg` dan `ffprobe` tidak diperlukan. Untuk membangun, hanya dibutuhkan Go (versinya ada di `go.mod`) dan `make` (tersedia di semua platform yang didukung; pada sistem minimal, dapat dipasang melalui package manager).
Makefile adalah entry point build yang kanonis: ia menetapkan lingkungan hermetik yang sama seperti yang digunakan CI (mode modul offline, toolchain lokal, tanpa cgo) dan menemukan toolchain Go secara otomatis.

```bash
make build      # kompilasi semuanya dan hasilkan ./procreepy yang dapat dijalankan
make check      # gofmt + build + vet + seluruh test suite
```

`make build` menyematkan `dev-<short sha>` (di luar checkout Git: `dev-nogit`)
ke binary sehingga `procreepy --version` menunjukkan asal commit-nya (tarball
rilis menggunakan tag sebagai gantinya). Tanpa `make`, padanan langsungnya
adalah `go build ./... && go build -o procreepy ./cmd/procreepy` (binary
tersebut melaporkan `dev`, atau `dev-<commit>` bila stamping VCS tersedia).

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
Semua build bersifat statis (tanpa cgo): binary Linux berjalan pada distro apa pun terlepas dari versi glibc. Pipeline CI (GitLab dan GitHub) membangun target yang sama pada setiap commit dan, karena Windows adalah platform utama, menjalankan seluruh test suite terhadap binary `windows/amd64` yang sebenarnya: secara natif di hosted runner GitHub, dan di bawah Wine pada GitLab yang hanya memiliki runner Linux (lihat `ci/windows-wine/`); job `dist` menerbitkan tarball beserta manifest `SHA256SUMS`, dan job `repro:*` membuktikan binary dapat direproduksi bit demi bit.

- Linux/macOS: tidak ada langkah instalasi, jalankan binary secara langsung.
- Windows: binary tidak ditandatangani, jadi SmartScreen mungkin menampilkan “Protected your PC” — pilih **More info → Run anyway**.

## Menjalankan dalam kontainer

Setiap tag rilis juga menerbitkan image multi-arsitektur (`linux/amd64`, `linux/arm64`) ke registry proyek, sehingga CLI berjalan tanpa toolchain Go dan tanpa membongkar tarball. Ini mencakup Apple Silicon juga: Docker Desktop dan `podman machine` menjalankan mesin virtual `linux/arm64` di Mac seri M, jadi varian arm64 dipilih otomatis — tanpa flag `--platform` dan tanpa emulasi.

```bash
# satu berkas — pasang direktori saat ini dan gunakan path di dalamnya
docker run --rm -v "$PWD":/data -w /data \
  registry.gitlab.com/po1nt-1/procreepy:latest artwork.procreate artwork.mp4

# mode batch: satu folder masuk, satu folder keluar
docker run --rm -v "$PWD":/data -w /data \
  registry.gitlab.com/po1nt-1/procreepy:latest input/ output/

# stdin → stdout tidak memerlukan mount sama sekali
docker run --rm -i registry.gitlab.com/po1nt-1/procreepy:latest - \
  < artwork.procreate > artwork.mp4
```

`podman` menggantikan `docker` apa adanya. Untuk mematok versi gunakan `:1.2.3` alih-alih `:latest` (tag image tidak memakai awalan `v`); `latest` tidak pernah dipindahkan ke tag prarilis.

- **Kepemilikan berkas.** Image berjalan sebagai uid 65532 (`distroless/static:nonroot`). Pada host Linux, tambahkan `--user "$(id -u):$(id -g)"` agar berkas keluaran menjadi milik Anda; Docker Desktop di macOS memetakan kepemilikan mount sendiri dan tidak memerlukan apa pun.
- **Tidak ada shell di dalam.** Image hanya memuat binary statis, jadi `docker run … --help` atau `… --verify file.procreate` berfungsi, tetapi tidak ada `sh` untuk masuk ke dalamnya.
- **Berkas sementara** ditulis ke lapisan kontainer yang dapat ditulisi, bukan ke mount. Timelapse besar bisa membutuhkan ruang di sana; `--tmpdir /data/tmp` memindahkannya ke volume yang dipasang.
- **Proyek privat.** Akses registry mengikuti visibilitas proyek: proyek publik dapat di-pull secara anonim, jika tidak jalankan `docker login registry.gitlab.com` terlebih dahulu.

## Penggunaan

Satu berkas:

```bash
procreepy artwork.procreate artwork.mp4
procreepy artwork.procreate > artwork.mp4
cat artwork.procreate | procreepy - > artwork.mp4
procreepy --list artwork.procreate
procreepy --verify artwork.procreate
```
Keempat kombinasi `INPUT`/`OUTPUT` didukung: `FILE OUTPUT`, `FILE -`, `- OUTPUT`,
`- -`. Jika `OUTPUT` dihilangkan, tujuannya adalah stdout. Jika `OUTPUT` adalah
direktori yang sudah ada, video ditempatkan di sana dengan nama aslinya
(`procreepy art.procreate videos/` → `videos/art.mp4`).

### Mode batch: folder berisi `.procreate` → folder berisi video dan proyek

Skenarionya: `input/` penuh dengan berkas `.procreate`, dan hasilnya ingin
ditempatkan di `output/`:

```bash
procreepy input/
```

Setiap karya yang dikonversi menghasilkan sepasang — timelapse dan proyek yang
bisa diimpor kembali tanpanya:

```text
input/                               output/
├── Portrait of a Cat.procreate  →   ├── timelapses/Portrait of a Cat.mp4
│                                    ├── projects/Portrait of a Cat.procreepy.procreate
├── Landscape v2.procreate       →   ├── timelapses/Landscape v2.mp4
│                                    └── projects/Landscape v2.procreepy.procreate
└── No Timelapse.procreate              (dilewati, dengan peringatan — tidak ada yang ditulis)
```
- **Nama**: `<nama asli tanpa .procreate>`, dengan `.mp4` atau
  `.procreepy.procreate` ditambahkan. Spasi, karakter Sirilik, dan karakter
  khusus dipertahankan apa adanya; pada sistem berkas yang tidak membedakan
  huruf besar/kecil, nama yang hanya berbeda hurufnya mendapat awalan
  belakang `-2`, `-3`, …
- **Folder keluaran**: secara default `output/` relatif terhadap direktori
  saat ini, dan dibuat otomatis. Folder lain dapat diberikan sebagai argumen
  kedua: `procreepy input/ ~/Videos/procreate`.
- **Proyek ramping**: `projects/NAME.procreepy.procreate` adalah arsip yang
  sama tanpa anggota `video/segments/segment-N.mp4`; semua anggota lain
  dibawa byte demi byte (urutan, metode kompresi, dan timestamp). Proyek
  menyimpan waktu modifikasi sumbernya, sehingga impor ulang ke Procreate
  tidak mengacak galeri.
- **Menjalankan ulang aman**: masukan yang timelapse dan proyeknya sudah ada
  akan dilewati; pasangan setengah jadi dibangun ulang secara keseluruhan.
  Untuk membangun ulang semuanya: `--force` (`-f`).
- **`-r`** juga menelusuri subfolder; struktur mereka direplikasi di kedua
  pohon, sehingga nama yang sama di folder berbeda tidak bertabrakan.
- **Satu berkas yang rusak tidak menghentikan yang lain.** Berkas tanpa
  timelapse (perekaman dimatikan) adalah peringatan, bukan kesalahan: tidak
  ada yang ditulis untuknya. Berkas korup adalah kesalahan: muncul dalam
  ringkasan akhir, dan kode keluar menjadi `1`.
- **Himpunan atomik**: keluaran dari satu masukan (timelapse + proyek, dan
  PSD-nya dengan `--psd`) ditulis ke berkas sementara dan diterbitkan
  bersama-sama — semuanya atau tidak sama sekali. Eksekusi yang gagal tidak
  pernah meninggalkan video yatim di samping proyek yang hilang.
- Berkas tersembunyi (`._Foo.procreate`, yang ditinggalkan macOS saat menyalin)
  diabaikan.
- Berkas asli tidak pernah diubah.

Contoh keluaran (semuanya ke stderr; eksekusi berakhir dengan kode keluar
`1` karena berkas yang korup):

```text
level=INFO msg="batch conversion started" files=4 input=input/ timelapses=output/timelapses/ projects=output/projects/
level=ERROR msg="file conversion failed" input="input/Corrupt file.procreate" err="input is not a valid ZIP archive: input/Corrupt file.procreate (not a .procreate file, or truncated/corrupted)"
level=INFO msg=converted input="input/Landscape v2.procreate" timelapse="output/timelapses/Landscape v2.mp4" project="output/projects/Landscape v2.procreepy.procreate" removed_segments=17 video_size="6.7 MiB"
level=WARN msg="no timelapse video inside, skipped" input="input/No Timelapse.procreate"
level=INFO msg=converted input="input/Portrait of a Cat.procreate" timelapse="output/timelapses/Portrait of a Cat.mp4" project="output/projects/Portrait of a Cat.procreepy.procreate" removed_segments=18 video_size="4.1 MiB"
level=INFO msg="batch completed" converted=2 existed=0 no_video=1 failed=1
```

`removed_segments` adalah jumlah berkas segmen yang dikeluarkan dari proyek
ramping, dan `video_size` adalah ukuran total (terkompresi) mereka di dalam
arsip. Menjalankan perintah yang sama sekali lagi melaporkan, untuk setiap
pasangan yang sudah ada:

```text
level=INFO msg="skipped, outputs already exist (use --force to overwrite)" input="input/Landscape v2.procreate" timelapse="output/timelapses/Landscape v2.mp4" project="output/projects/Landscape v2.procreepy.procreate"
```

`--list` dan `--verify` juga menerima direktori dan menelusuri semua berkas di dalamnya.

### Ekspor PSD (`--psd`)

```bash
procreepy --psd input/ out/
```

Hanya masukan direktori. Di samping setiap pasangan yang dikonversi,
`out/psd/NAME.psd` ditulis, diterbitkan secara atomik bersama sisa himpunan:

```text
level=INFO msg="psd exported" input="input/Portrait of a Cat.procreate" psd="out-psd/psd/Portrait of a Cat.psd" layers=3
```

Apa yang ada di dalam PSD:

- pohon layer (grup, urutan), nama layer (Unicode), visibilitas,
  opasitas, mode blending, batas, dan status penguncian;
- piksel RGBA 8-bit untuk setiap layer, dikompresikan dengan PackBits;
- DPI dan profil ICC yang tertanam;
- komposit gabungan yang diambil apa adanya dari render ratakan milik
  Procreate (ketika itu hilang atau rusak, layer yang terlihat dikomposisi
  dalam mode Normal sebagai pendekatan).

Apa yang tidak ada — PSD adalah ekspor, bukan putaran balik tanpa kerugian:

- mask layer dan semantik clip tepat ke layer di bawahnya tidak bertahan;
- layer teks mempertahankan pikselnya, tetapi bukan data teksnya yang
  dapat disunting;
- alpha lurus (tidak pra-dikalikan) tidak dapat dipulihkan secara tepat:
  Procreate menyimpan ubin 8-bit yang pra-dikalikan, sehingga warna pinggir
  di tepi layer dapat sedikit berbeda;
- kanvas yang lebih lebar atau tinggi dari 30000 piksel ditolak mentah-mentah
  (batas format PSD, berlawanan dengan PSB).

Berkas `.procreate` tetap menjadi salinan induk; anggaplah PSD sebagai
potret untuk Photoshop dan importir lainnya.

### Diagnostik

```bash
procreepy --list artwork.procreate
```

```text
input: input/Portrait of a Cat.procreate
segments: 18

1  video/segments/segment-1.mp4
2  video/segments/segment-2.mp4
3  video/segments/segment-3.mp4
...
17 video/segments/segment-17.mp4
18 video/segments/segment-18.mp4
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
| `-h`, `--help` | tampilkan bantuan dan keluar |
| `--list` | urutkan segmen sesuai urutan pemutaran dan keluar |
| `--verify` | periksa setiap segmen; tidak membuat video keluaran |
| `-r`, `--recursive` | masukan direktori: telusuri juga subfolder |
| `-f`, `--force` | masukan direktori: timpa hasil yang sudah ada |
| `--strict` | perlakukan nomor segmen yang hilang sebagai kesalahan (default: peringatan) |
| `--psd` | masukan direktori: ekspor pula PSD berlayer untuk setiap karya |
| `--tmpdir DIR` | tempat menyimpan file sementara (default: `$TMPDIR`, lalu `/var/tmp`, lalu direktori sementara sistem) |
| `-q`, `--quiet` | hanya cetak peringatan dan kesalahan |
| `--version` | tampilkan nomor versi dan keluar |
| `--` | hentikan pengolahan opsi; perlakukan sisanya sebagai argumen posisional |

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
7. Apa pun yang dijanjikan eksekusi (MP4, proyek ramping, PSD) disiapkan di
   berkas sementara dan diterbitkan sebagai satu himpunan hanya setelah yang
   terakhir berhasil; dalam mode batch, masukan berikutnya dicoba apa pun
   hasilnya.
8. Berkas sementara (jika ada) selalu dihapus — saat berhasil, gagal, Ctrl+C,
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
  * Kekhususan Windows: penamaan ulang akhir memakai `MoveFileEx` dengan
    penggantian target yang sudah ada, sehingga `--force` dan regenerasi set
    yang tidak lengkap menggantikan keluaran yang ada di tempatnya, sama
    seperti di Unix. Secara ketat ia tidak se-atomik rename POSIX; perbedaan
    praktis hanya tampak dalam satu kasus — jika berkas target masih dibuka
    oleh program lain (misalnya pemutar media yang masih memegang MP4
    sebelumnya), penamaan ulang ditolak dengan pesan error yang mudah dibaca
    `Access is denied`, berkas lama tetap utuh, dan eksekusi ulang setelah
    program itu ditutup akan berhasil.
  * stdout tidak pernah tercampur dengan teks. Semua baris log
    (`level=INFO`/`WARN`/`ERROR`, satu record terstruktur key=value per baris)
    dikirim ke stderr. Satu-satunya pengecualian adalah laporan
    `--list`/`--verify`, di mana stdout adalah hasilnya. Jika stdout adalah terminal,
    utilitas menolak menuliskan MP4 biner ke sana.

### Warna konsol

Ketika stderr adalah terminal interaktif dan variabel lingkungan `NO_COLOR`
tidak ditetapkan, token tingkat `WARN` dan `ERROR` diberi warna (kuning dan
merah tebal); `INFO` tetap polos. Pipe, redirect, CI, dan uji memelihara
format plain byte demi byte, sehingga tidak ada yang terskrip yang berubah.
Secara sadar tidak ada flag `--color`.

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
| 2 | argumen salah (termasuk `--psd` dengan satu berkas, atau direktori hasil yang diarahkan ke stdout); OUTPUT adalah berkas yang sama dengan INPUT; stdout adalah terminal |
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

Jalur timelapse tidak mengurai `Document.archive` (NSKeyedArchive), tidak
menyentuh `*.lz4`, tidak memulihkan layer, dan tidak merender gambar; `--psd`
memang mengurai dokumen — untuk ekspor yang dijelaskan di atas, dengan
catatan keakuratan dari bagian tersebut. Jika timelapse tidak direkam dalam
berkas, utilitas ini tidak dapat memulihkannya dari riwayat gambar. Catatan:
`lz4 -t` pada `.lz4` yang diambil dari `.procreate` bukan pemeriksaan
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
