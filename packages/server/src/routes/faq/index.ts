import express, { Request, Response } from 'express'
import fs from 'fs'
import path from 'path'

const router = express.Router()
const FAQ_FILE = path.join(process.env.HOME || '/root', '.flowise', 'faq.json')

const DEFAULT_FAQ = [
    {
        id: '1',
        category: 'WhatsApp Bot',
        question: 'Bagaimana cara menghubungkan WhatsApp bot?',
        answer: 'Buka menu WA Session di sidebar kiri. Klik "Scan QR Code" dan scan menggunakan WhatsApp → Linked Devices → Link a Device. Bot akan aktif setelah scan berhasil.'
    },
    {
        id: '2',
        category: 'WhatsApp Bot',
        question: 'Nomor WhatsApp mana yang digunakan untuk bot?',
        answer: 'Bot menggunakan nomor WhatsApp yang di-link via menu WA Session. Buka WA Session → lihat status Connected untuk melihat nomor aktif. Nomor ini khusus untuk bot dan tidak boleh digunakan untuk chat biasa saat bot aktif.'
    },
    {
        id: '3',
        category: 'WhatsApp Bot',
        question: 'Bagaimana cara melihat status koneksi bot?',
        answer: 'Buka menu WA Session di sidebar. Status "Connected" (hijau) berarti bot aktif. Status "Offline" berarti perlu scan QR ulang.'
    },
    {
        id: '4',
        category: 'Upload Data',
        question: 'Bagaimana cara upload penawaran supplier via WhatsApp/Telegram?',
        answer: 'Kirim file Excel/CSV ke bot owner (WhatsApp atau Telegram). Bot akan bertanya tujuan file tersebut. Pilih "1 Penawaran Supplier", lalu masukkan nama supplier dan mata uang (contoh: SUPPLIER: PT ABC, CURRENCY: USD). Bot akan upload otomatis ke sistem TRADE.'
    },
    {
        id: '5',
        category: 'Upload Data',
        question: 'Format file apa saja yang didukung untuk upload?',
        answer: 'Format yang didukung: Excel (.xlsx, .xls) dan CSV (.csv). Untuk penawaran supplier, kolom minimal: kode barang, nama barang, harga, satuan.'
    },
    {
        id: '6',
        category: 'Upload Data',
        question: 'Bagaimana cara import data perdagangan?',
        answer: 'Kirim file Excel/CSV ke bot owner → pilih "2 Data Perdagangan". Bot akan menampilkan preview validasi kolom. Ketik "ya" untuk konfirmasi import, atau "batal" untuk membatalkan.'
    },
    {
        id: '7',
        category: 'Upload Data',
        question: 'Apa itu Permintaan Barang dan bagaimana cara import?',
        answer: 'Permintaan Barang adalah data barang yang belum tercatat di sistem. Kirim file ke bot → pilih "3 Permintaan Barang". Setelah import, sistem akan otomatis melakukan mapping ke katalog produk.'
    },
    {
        id: '8',
        category: 'Chatflow & AI',
        question: 'Apa itu Chatflow dan Agentflow?',
        answer: 'Chatflow adalah alur percakapan AI yang sudah dikonfigurasi. Agentflow adalah alur AI yang bisa menggunakan tools dan mengambil keputusan sendiri. Keduanya bisa dibuat secara visual di menu Chatflows/Agentflows.'
    },
    {
        id: '9',
        category: 'Chatflow & AI',
        question: 'Bagaimana cara mengganti model AI yang digunakan?',
        answer: 'Buka Chatflow → klik node model AI (contoh: ChatOpenAI) → ganti model di field "Model Name". Pastikan API key sudah diisi di menu Credentials.'
    },
    {
        id: '10',
        category: 'Sistem & Maintenance',
        question: 'Apa yang harus dilakukan jika bot tidak merespons?',
        answer: '1. Cek WA Session — pastikan status Connected. 2. Jika Offline, scan QR ulang. 3. Jika masih bermasalah, hubungi admin untuk restart service bot.'
    },
    {
        id: '11',
        category: 'Sistem & Maintenance',
        question: 'Siapa yang bisa mengakses halaman Agentic OB ini?',
        answer: 'Hanya admin yang memiliki akun login. Untuk request akses baru, hubungi administrator sistem.'
    }
]

function loadFaq() {
    try {
        if (fs.existsSync(FAQ_FILE)) {
            return JSON.parse(fs.readFileSync(FAQ_FILE, 'utf8'))
        }
    } catch {}
    return DEFAULT_FAQ
}

function saveFaq(data: any[]) {
    const dir = path.dirname(FAQ_FILE)
    if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true })
    fs.writeFileSync(FAQ_FILE, JSON.stringify(data, null, 2), 'utf8')
}

router.get('/', (req: Request, res: Response) => {
    res.json(loadFaq())
})

router.post('/', (req: Request, res: Response) => {
    const data = req.body
    if (!Array.isArray(data)) {
        res.status(400).json({ error: 'body must be array' })
        return
    }
    saveFaq(data)
    res.json({ ok: true })
})

export default router
