import express from 'express'
import http from 'http'

const router = express.Router()

// Proxy ke log-search service di port 8200
router.get('/search', (req, res) => {
    const q = req.query.q as string
    const date = req.query.date as string
    if (!q) return res.status(400).json({ error: 'query required' })

    const path = `/logs/search?q=${encodeURIComponent(q)}&date=${encodeURIComponent(date || '')}`
    const options = {
        hostname: '127.0.0.1',
        port: 8200,
        path,
        method: 'GET',
        headers: { 'X-Internal-Key': 'ob-jurnal-internal-2026' }
    }

    const proxyReq = http.request(options, (proxyRes) => {
        res.setHeader('Content-Type', 'application/json')
        res.status(proxyRes.statusCode || 200)
        proxyRes.pipe(res)
    })
    proxyReq.on('error', (e) => res.status(503).json({ error: 'log-search unavailable: ' + e.message }))
    proxyReq.end()
})

export default router
