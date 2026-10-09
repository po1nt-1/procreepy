# procreepy

Mengubah timelapse yang sudah direkam Procreate di dalam berkas `.procreate`
Anda menjadi MP4 biasa.

- Tanpa enkode ulang: frame-nya disalin, jadi videonya mempertahankan kualitas Procreate.
- Berkas `.procreate` asli Anda tidak pernah diubah.
- Satu berkas atau satu folder penuh sekaligus.
- Satu program saja. Tanpa ffmpeg, tanpa Python, tanpa akun, tanpa konfigurasi.
- Berjalan offline di Windows, macOS, dan Linux.

## Apakah ini untuk Anda?

Gunakan procreepy jika:

- Anda punya berkas `.procreate`;
- **perekaman timelapse aktif** saat Anda menggambar (di Procreate ini aktif
  secara bawaan);
- Anda ingin timelapse itu sebagai MP4 untuk diunggah, diedit, atau disimpan;
- atau Anda ingin membuang data timelapse dari proyek agar ukurannya lebih kecil.

procreepy **tidak bisa**:

- membuat timelapse yang memang tidak pernah direkam — ia hanya mengekstrak yang
  sudah ada;
- menyusun ulang timelapse dari lapisan atau riwayat undo Anda;
- memperbaiki berkas `.procreate` yang rusak;
- mengekspor gambar jadi dari karya Anda (ia bisa mengekspor PSD berlapis, lihat
  [Ekspor ke PSD](#ekspor-ke-psd)).

Tidak yakin berkas Anda punya timelapse?
[Periksa dulu](#memeriksa-berkas-sebelum-konversi) — satu perintah, dan tidak
membuat apa pun.

## Apa yang Anda dapat

Satu berkas masuk, satu video keluar:

```text
my-art.procreate  →  procreepy  →  my-art.mp4
```

Satu folder masuk; keluar sebuah pohon video dan satu arsip proyek:

```text
input/                          input_procreepy/
├── Cat.procreate         →     ├── mp4/
├── Landscape.procreate   →     │   ├── Cat.mp4
└── Sketch.procreate      →     │   ├── Landscape.mp4
                                │   └── Sketch.mp4
                                └── procreate.zip   (proyek ramping)
```

- `mp4/` berisi videonya.
- `procreate.zip` berisi salinan setiap karya **dengan timelapse-nya dibuang**:
  jauh lebih kecil, dan bisa Anda impor kembali ke Procreate. Semua isi proyek
  yang lain dipertahankan bit demi bit. Ini zip biasa berisi folder `procreate/`
  di dalamnya, siap dipindahkan ke iPad; gunakan `--no-zip` untuk membiarkan
  folder `procreate/` itu di disk alih-alih arsipnya.
- Folder keluaran dinamai mengikuti folder masukan (`input/` → `input_procreepy/`)
  dan dibuat di direktori tempat Anda menjalankan perintah. Beri jalur kedua untuk
  memilihnya sendiri.
- Setiap keluaran mempertahankan tanggal berkas asalnya, sehingga mengimpor ulang
  proyek ke Procreate tidak mengacak galeri Anda.
- `input/` tetap persis seperti sebelumnya.

## Pemasangan

Unduh program siap pakai untuk sistem Anda dari halaman rilis — Anda tidak perlu
Go atau alat build apa pun.

- GitLab: https://gitlab.com/po1nt-1/procreepy/-/releases
- GitHub: https://github.com/po1nt-1/procreepy/releases

Pilih berkas yang sesuai dengan komputer Anda:

| Sistem Anda | Unduhan |
|---|---|
| Windows (sebagian besar PC) | `procreepy_<version>_windows_amd64.zip` |
| Windows pada ARM | `procreepy_<version>_windows_arm64.zip` |
| Mac dengan Apple Silicon (M1–M5) | `procreepy_<version>_darwin_arm64.tar.gz` |
| Mac dengan Intel | `procreepy_<version>_darwin_amd64.tar.gz` |
| Linux (sebagian besar PC) | `procreepy_<version>_linux_amd64.tar.gz` |
| Linux pada ARM (Raspberry Pi, Graviton) | `procreepy_<version>_linux_arm64.tar.gz` |
| Linux pada ARM 32-bit | `procreepy_<version>_linux_arm.tar.gz` |

Di Mac, menu Apple → "Tentang Mac Ini" menunjukkan apakah Anda memakai Apple
Silicon atau Intel.

### Windows

1. Unduh `procreepy_<version>_windows_amd64.zip`.
2. Klik kanan berkas unduhan → **Extract All** → pilih folder yang mudah Anda
   temukan lagi, misalnya `Downloads\procreepy`.
3. Buka folder itu, klik bilah alamat di atas, ketik `cmd`, lalu tekan Enter.
   Jendela hitam Command Prompt terbuka di folder tersebut.
4. Taruh sebuah berkas `.procreate` di folder yang sama dan jalankan:

   ```text
   procreepy.exe "My Artwork.procreate" "My Artwork.mp4"
   ```

   Tanda kutip hanya perlu jika namanya mengandung spasi.

**Tentang peringatan SmartScreen.** Program ini tidak ditandatangani dengan
sertifikat Microsoft berbayar, jadi saat pertama dijalankan Windows mungkin
menampilkan jendela biru "Windows protected your PC" dan menyebutnya
"unrecognized app". Ini bukan laporan virus — Windows menampilkannya untuk
program apa pun yang belum cukup sering dilihatnya. Untuk melanjutkan, klik
**More info**, lalu tombol **Run anyway** yang muncul. Jika Anda lebih suka
tidak melakukannya, gunakan [image kontainer](#docker--podman).

### macOS

1. Unduh `.tar.gz` untuk chip Anda (`darwin_arm64` untuk Apple Silicon,
   `darwin_amd64` untuk Intel).
2. Buka Terminal (Applications → Utilities → Terminal) dan masuk ke folder
   unduhan:

   ```bash
   cd ~/Downloads
   ```

3. Ekstrak dan izinkan agar bisa dijalankan:

   ```bash
   tar -xzf procreepy_*_darwin_*.tar.gz
   xattr -d com.apple.quarantine ./procreepy
   ```

   Baris `xattr` menghapus tanda karantina unduhan. Tanpa itu macOS menolak
   menjalankan program, karena ia tidak dinotarisasi oleh Apple.

4. Konversikan sebuah berkas:

   ```bash
   ./procreepy "My Artwork.procreate" "My Artwork.mp4"
   ```

Agar bisa mengetik `procreepy` dari mana saja, pindahkan ke PATH:
`sudo mv ./procreepy /usr/local/bin/`.

### Linux

```bash
tar -xzf procreepy_*_linux_amd64.tar.gz
./procreepy artwork.procreate artwork.mp4
```

Program ini di-link secara statis, jadi berjalan di distro apa pun tanpa
bergantung pada versi glibc. Untuk memasangnya bagi semua pengguna:
`sudo install -m 755 procreepy /usr/local/bin/`.

### Docker / Podman

Image kontainer diterbitkan pada setiap rilis, untuk `linux/amd64` dan
`linux/arm64`. Di Mac Apple Silicon, varian arm64 dipilih otomatis.

```bash
docker run --rm -v "$PWD":/data -w /data \
  registry.gitlab.com/po1nt-1/procreepy:latest artwork.procreate artwork.mp4
```

`podman` menggantikan `docker` apa adanya. Untuk mematok versi gunakan `:0.3.0`
alih-alih `:latest` (tag image tidak memakai awalan `v`). Rincian soal
kepemilikan berkas dan lainnya ada di [usage.md](usage.md#container-usage).

### Build dari sumber

Hanya perlu jika Anda ingin mengubah kodenya. Lihat
[development.md](development.md).

### Memverifikasi unduhan (opsional)

Setiap rilis juga menerbitkan `CHECKSUMS.txt`. Untuk memastikan unduhan tiba utuh,
tampilkan hash berkas Anda dan bandingkan dengan baris yang sesuai:

```bash
sha256sum procreepy_0.3.0_linux_amd64.tar.gz    # Linux
shasum -a 256 procreepy_0.3.0_darwin_arm64.tar.gz   # macOS
grep darwin_arm64 CHECKSUMS.txt                 # nilai yang diharapkan
```

Di Windows: `certutil -hashfile procreepy_0.3.0_windows_amd64.zip SHA256`.

Kedua nilai harus sama. Langkah ini opsional — ia mendeteksi unduhan terpotong
atau dirusak, tidak lebih.

## Mengonversi satu berkas

```bash
procreepy artwork.procreate artwork.mp4
```

Hasil:

```text
artwork.mp4
```

Berkas asli `artwork.procreate` tidak diubah. Jika `artwork.mp4` sudah ada, ia
diganti — tetapi hanya setelah video baru selesai ditulis sepenuhnya.

Anda juga bisa memberi folder sebagai tujuan dan membiarkan procreepy menamai
berkasnya:

```bash
procreepy artwork.procreate videos/
```

Hasil: `videos/artwork.mp4`. Foldernya harus sudah ada.

## Mengonversi satu folder

```bash
procreepy input/
```

Membaca setiap `.procreate` di `input/` dan menulis ke `input_procreepy/`,
seperti pada [Apa yang Anda dapat](#apa-yang-anda-dapat). Untuk memilih tujuan
sendiri:

```bash
procreepy input/ ~/Videos/timelapses
```

Untuk menyertakan subfolder (strukturnya dicerminkan di keluaran):

```bash
procreepy -r input/ output/
```

Apa yang terjadi selama berjalan:

- Kemajuan dilaporkan per berkas, satu baris masing-masing.
- Berkas yang timelapse-nya tidak pernah direkam tetap menghasilkan proyek
  rampingnya (dan PSD-nya dengan `--psd`) — hanya videonya yang dilewati, dengan
  catatan, dan proses berlanjut.
- Berkas yang rusak dilaporkan sebagai galat, proses tetap lanjut ke sisanya, dan
  perintah berakhir dengan kode keluar `1` agar skrip bisa menyadarinya.
- Menjalankan perintah yang sama dua kali tidak mengulang pekerjaan yang sudah
  selesai: karya yang hasilnya sudah ada akan dilewati. Tambahkan `-f` untuk
  membangunnya ulang.
- Setelah proses tanpa kegagalan, proyek-proyek ramping dikemas ke dalam
  `procreate.zip` dan folder `procreate/` dihapus. Proses dengan kegagalan apa
  pun membiarkan foldernya tetap belum dikemas, agar bisa Anda periksa dan
  lanjutkan. Gunakan `--no-zip` untuk selalu menyimpan foldernya.

## Memeriksa berkas sebelum konversi

Kedua perintah ini tidak membuat video dan tidak mengubah apa pun.

**Apakah ada timelapse di berkas ini, dan berapa panjangnya?**

```bash
procreepy --list artwork.procreate
```

```text
input: artwork.procreate
segments: 18

1  video/segments/segment-1.mp4
2  video/segments/segment-2.mp4
...
```

`--list` membaca daftar isi berkas. Seketika, dan memberi tahu apakah timelapse
memang ada.

**Apakah konversinya benar-benar akan berhasil?**

```bash
procreepy --verify artwork.procreate
```

`--verify` melangkah lebih jauh: membaca setiap segmen timelapse, memeriksa
kerusakan, dan memastikan segmen-segmen itu bisa disambung tanpa enkode ulang.
Lebih lambat dari `--list`, dan merupakan jawaban jujur atas "apakah ini akan
terkonversi dengan bersih?".

Keduanya juga menerima folder, lalu melaporkan setiap berkas di dalamnya.

## Ekspor ke PSD

```bash
procreepy --psd input/ output/
```

Di samping setiap video dan proyek ringkas, ditulis `output/psd/NAME.psd`: berkas
Photoshop berlapis yang bisa dibuka di Photoshop, Affinity Photo, GIMP, dan
sejenisnya.

`--psd` bekerja **hanya dengan folder sebagai masukan**. Dengan satu berkas,
perintah berhenti dengan `--psd needs a directory INPUT; it writes into
OUTPUT/psd/`.

PSD adalah hasil ekspor, bukan salinan sempurna. Ia mempertahankan pohon lapisan,
struktur dan urutan grup, nama, visibilitas, opasitas, mode blend, dan gambarnya
sendiri; ia **tidak** mempertahankan mask lapisan, hubungan clipping, atau teks
yang bisa diedit. Baca
[apa yang disimpan dan apa yang hilang pada PSD](usage.md#export-a-psd) sebelum
memakainya untuk pekerjaan akhir. Simpan berkas `.procreate` sebagai salinan
induk.

PSD juga menyematkan gambar pratinjau kecil, sehingga aplikasi yang membacanya
(Photoshop, Affinity, GIMP) menampilkan thumbnail. Ini **tidak** dengan
sendirinya membuat Windows Explorer menggambar thumbnail: Explorer memerlukan
penangan thumbnail terdaftar untuk `.psd`, yang tidak disertakan Windows
(Photoshop atau paket seperti SageThumbs menyediakannya), dan untuk `.procreate`
tidak ada sama sekali.

## Apa yang terjadi pada berkas Anda

- **Berkas asli Anda tidak pernah diubah.** procreepy membuka `.procreate` dalam
  mode baca saja. Semua yang dihasilkannya ditulis di tempat lain.
- **Tidak ada yang tertulis separuh jalan.** Setiap hasil dibangun lebih dulu di
  berkas sementara dan dipindahkan ke tempatnya hanya setelah lengkap. Proses
  yang terputus atau gagal tidak pernah meninggalkan video rusak dan tidak
  merusak berkas yang sudah ada.
- **Proses folder menerbitkan per karya, sebagai satu set.** Video, proyek
  ringkas, dan PSD satu karya muncul bersama atau tidak muncul sama sekali —
  Anda tidak akan mendapat video tanpa proyeknya.
- **Tanggal ikut dibawa.** Setiap keluaran — video, proyek ramping, dan PSD —
  diberi tanggal modifikasi berkas `.procreate` asalnya (dan, di Windows, tanggal
  pembuatannya juga), sehingga proyek yang diimpor ulang ke Procreate tetap pada
  tempatnya di galeri.
- **Menjalankan ulang itu aman.** Karya yang sudah selesai dilewati, baik proyek
  masih berupa folder maupun sudah dikemas ke `procreate.zip`. Satu set yang
  tertinggal tidak lengkap karena proses terputus akan dibangun ulang seluruhnya.
  `-f` membangun ulang semuanya.
- **Mengonversi satu berkas mengganti tujuannya** bila sudah ada, setelah video
  baru selesai ditulis.
- **Berkas besar butuh ruang sementara.** Timelapse besar disusun melalui berkas
  sementara. Jika ruangnya habis, arahkan `--tmpdir` ke disk yang lebih lega.

## Jika ada yang salah

| Yang Anda lihat | Artinya |
|---|---|
| `procreepy: command not found` | Anda tidak berada di folder tempat mengekstraknya; di macOS/Linux pakai `./procreepy`. |
| `no video/segments in the archive` | Tidak ada timelapse yang direkam di berkas itu. Tidak bisa dipulihkan. |
| `input is not a valid ZIP archive` | Bukan berkas `.procreate`, atau unduhan/salinannya terpotong. |
| `segment ... is corrupted inside the archive` | Data timelapse-nya rusak. |
| `segments are incompatible` | Timelapse direkam melewati pergantian kanvas atau kualitas, sehingga tidak bisa disambung tanpa enkode ulang. |
| `refusing to write video data to a terminal` | Berikan nama berkas tujuan, atau alihkan dengan `> out.mp4`. |
| `Windows protected your PC` | Lihat [catatan SmartScreen](#windows). |
| `no space left` / galat penulisan | Gunakan `--tmpdir` pada disk dengan ruang lebih banyak. |

Setiap kasus di atas, dengan gejala tepatnya dan apa yang harus dilakukan, ada di
[troubleshooting.md](troubleshooting.md).

## Rujukan perintah

```text
procreepy [options] INPUT [OUTPUT]
```

`INPUT` adalah berkas `.procreate`, folder berisi berkas-berkas itu, atau `-`
untuk masukan standar. `OUTPUT` adalah nama berkas, folder, atau `-` untuk
keluaran standar. Untuk satu berkas, `OUTPUT` yang dikosongkan berarti video
ditulis ke keluaran standar; untuk folder, bawaannya `<INPUT>_procreepy/` di
direktori saat ini.

| Opsi | Fungsinya | Berlaku untuk |
|---|---|---|
| `-h`, `--help` | tampilkan bantuan lalu keluar | selalu |
| `--version` | tampilkan versi lalu keluar | selalu |
| `--list` | daftar segmen timelapse; tidak menulis video | berkas atau folder |
| `--verify` | periksa setiap segmen; tidak menulis video | berkas atau folder |
| `-r`, `--recursive` | proses subfolder juga | hanya masukan folder |
| `-f`, `--force` | timpa hasil yang sudah ada | hanya masukan folder |
| `--psd` | ekspor juga PSD berlapis per karya | hanya masukan folder |
| `--no-zip` | biarkan proyek sebagai folder `procreate/` alih-alih mengemas `procreate.zip` | hanya masukan folder |
| `--strict` | anggap nomor segmen yang bolong sebagai galat, bukan peringatan | berkas atau folder |
| `--tmpdir DIR` | tempat menaruh berkas sementara | selalu |
| `-q`, `--quiet` | cetak hanya peringatan dan galat | selalu |
| `--` | hentikan pembacaan opsi; sisanya dianggap nama berkas | selalu |

Rujukan lengkap dengan contoh, format keluaran, dan kode keluar:
[usage.md](usage.md).

## Cara kerjanya

Berkas `.procreate` adalah arsip ZIP. Ketika perekaman timelapse aktif, Procreate
menyimpan video yang sudah jadi di dalamnya, terbagi menjadi potongan bernomor
(`video/segments/segment-1.mp4`, `segment-2.mp4`, …). procreepy membaca potongan
itu langsung dari arsip, mengurutkannya secara numerik, memastikan codec dan
kanvasnya sama, lalu menjahitnya menjadi satu MP4 dengan menyalin frame tanpa
diubah. Tidak ada yang dirender dan tidak ada yang dienkode ulang — itulah
sebabnya cepat dan tanpa kehilangan kualitas.

Jalur timelapse tidak pernah melihat lapisan Anda. Hanya `--psd` yang membaca
karyanya sendiri.

Rincian — penyusunan MP4, penulisan atomik, strategi berkas sementara, kesetiaan
PSD: [how-it-works.md](how-it-works.md).

## Pengembangan

Build, pengujian, cakupan, kompilasi silang, rilis, dan CI:
[development.md](development.md).

Prasyaratnya adalah Go (versinya ada di [`go.mod`](../go.mod)) dan `make`. Proyek
ini tidak punya dependensi pihak ketiga.

```bash
make check   # pemeriksaan format + build + vet + seluruh rangkaian tes
```

## Lisensi

Apache License 2.0 — lihat [LICENSE](../LICENSE).

## Bahasa

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
