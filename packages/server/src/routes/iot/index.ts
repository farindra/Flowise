import express from 'express'
import http from 'http'

const router = express.Router()

function proxy(path: string, method: string, req: express.Request, res: express.Response, body?: string) {
    const options = {
        hostname: '127.0.0.1',
        port: 8084,
        path,
        method,
        headers: {
            'X-Internal-Key': 'ob-iot-internal-2026',
            'Content-Type': 'application/json',
            ...(body ? { 'Content-Length': String(Buffer.byteLength(body)) } : {})
        }
    }
    const proxyReq = http.request(options, (proxyRes) => {
        res.setHeader('Content-Type', 'application/json')
        res.status(proxyRes.statusCode || 200)
        proxyRes.pipe(res)
    })
    proxyReq.on('error', (e) => res.status(503).json({ error: 'IoT service unavailable: ' + e.message }))
    if (body) proxyReq.write(body)
    proxyReq.end()
}

router.get('/zones', (req, res) => proxy('/api/zones', 'GET', req, res))
router.get('/alerts', (req, res) => {
    const qs = new URLSearchParams(req.query as Record<string, string>).toString()
    proxy(`/api/alerts${qs ? '?' + qs : ''}`, 'GET', req, res)
})
router.get('/readings/history', (req, res) => {
    const qs = new URLSearchParams(req.query as Record<string, string>).toString()
    proxy(`/api/readings/history?${qs}`, 'GET', req, res)
})
router.put('/alerts/:id/resolve', (req, res) => proxy(`/api/alerts/${req.params.id}/resolve`, 'PUT', req, res))
router.post('/readings', (req, res) => {
    const body = JSON.stringify(req.body)
    proxy('/api/readings', 'POST', req, res, body)
})

export default router
