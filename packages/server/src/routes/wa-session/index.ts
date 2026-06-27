import express, { Request, Response } from 'express'
import https from 'https'
import http from 'http'

const router = express.Router()

const WA_GATEWAY_URL = process.env.WA_GATEWAY_URL || 'http://127.0.0.1:8100'
const WA_GATEWAY_TOKEN = process.env.WA_GATEWAY_TOKEN || 'ob-wa-qr-2026'

function proxyGet(path: string) {
    return async (req: Request, res: Response) => {
        const url = `${WA_GATEWAY_URL}${path}?token=${WA_GATEWAY_TOKEN}`
        const lib = url.startsWith('https') ? https : http
        const proxyReq = lib.get(url, (proxyRes) => {
            res.status(proxyRes.statusCode || 200)
            const ct = proxyRes.headers['content-type'] || 'application/json'
            res.setHeader('Content-Type', ct)
            proxyRes.pipe(res)
        })
        proxyReq.on('error', (err) => {
            res.status(502).json({ error: 'WA gateway unreachable', detail: err.message })
        })
    }
}

function proxyPost(path: string) {
    return async (req: Request, res: Response) => {
        const url = `${WA_GATEWAY_URL}${path}?token=${WA_GATEWAY_TOKEN}`
        const lib = url.startsWith('https') ? https : http
        const proxyReq = lib.request(url, { method: 'POST' }, (proxyRes) => {
            res.status(proxyRes.statusCode || 200)
            res.setHeader('Content-Type', proxyRes.headers['content-type'] || 'application/json')
            proxyRes.pipe(res)
        })
        proxyReq.on('error', (err) => {
            res.status(502).json({ error: 'WA gateway unreachable', detail: err.message })
        })
        proxyReq.end()
    }
}

// GET /api/v1/wa-session/status
router.get('/status', proxyGet('/status'))

// GET /api/v1/wa-session/qr  → returns QR image PNG
router.get('/qr', proxyGet('/qr'))

// POST /api/v1/wa-session/logout
router.post('/logout', proxyPost('/logout'))

// POST /api/v1/wa-session/pair-phone?phone=628xxx
router.post('/pair-phone', async (req: Request, res: Response) => {
    const phone = req.query.phone as string
    if (!phone) {
        res.status(400).json({ error: 'missing phone query param' })
        return
    }
    const url = `${WA_GATEWAY_URL}/pair-phone?token=${WA_GATEWAY_TOKEN}&phone=${encodeURIComponent(phone)}`
    const lib = url.startsWith('https') ? https : http
    const proxyReq = lib.get(url, (proxyRes) => {
        res.status(proxyRes.statusCode || 200)
        res.setHeader('Content-Type', proxyRes.headers['content-type'] || 'application/json')
        proxyRes.pipe(res)
    })
    proxyReq.on('error', (err) => {
        res.status(502).json({ error: 'WA gateway unreachable', detail: err.message })
    })
})

export default router
