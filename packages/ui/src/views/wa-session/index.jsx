import { useEffect, useState, useCallback } from 'react'
import {
    Box,
    Button,
    Card,
    CardContent,
    Chip,
    CircularProgress,
    Divider,
    Stack,
    TextField,
    Typography,
    Alert,
    Tooltip
} from '@mui/material'
import {
    IconRefresh,
    IconPlugConnected,
    IconPlugConnectedX,
    IconPhone,
    IconQrcode,
    IconDeviceMobile
} from '@tabler/icons-react'
import MainCard from '@/ui-component/cards/MainCard'
import { baseURL } from '@/store/constant'
import apiClient from '@/api/client'

export default function WASession() {
    const [status, setStatus] = useState(null)
    const [loading, setLoading] = useState(true)
    const [qrTimestamp, setQrTimestamp] = useState(Date.now())
    const [phone, setPhone] = useState('')
    const [pairingCode, setPairingCode] = useState('')
    const [pairingLoading, setPairingLoading] = useState(false)
    const [logoutLoading, setLogoutLoading] = useState(false)
    const [error, setError] = useState('')
    const [success, setSuccess] = useState('')

    const fetchStatus = useCallback(async () => {
        try {
            const res = await apiClient.get('/wa-session/status')
            setStatus(res.data)
        } catch {
            setStatus(null)
        } finally {
            setLoading(false)
        }
    }, [])

    useEffect(() => {
        fetchStatus()
        const interval = setInterval(fetchStatus, 5000)
        return () => clearInterval(interval)
    }, [fetchStatus])

    // Auto-refresh QR image every 30s when not logged in
    useEffect(() => {
        if (status && !status.logged_in) {
            const t = setInterval(() => setQrTimestamp(Date.now()), 30000)
            return () => clearInterval(t)
        }
    }, [status])

    const handlePairPhone = async () => {
        const cleaned = phone.replace(/\D/g, '').replace(/^0/, '62').replace(/^\+/, '')
        if (!cleaned) { setError('Masukkan nomor telepon'); return }
        setError('')
        setSuccess('')
        setPairingCode('')
        setPairingLoading(true)
        try {
            const res = await apiClient.post(`/wa-session/pair-phone?phone=${cleaned}`)
            if (res.data?.pairing_code) {
                setPairingCode(res.data.pairing_code)
                setSuccess('Kode pairing berhasil didapat!')
            } else {
                setError(res.data?.error || 'Gagal mendapat kode pairing')
            }
        } catch (e) {
            setError(e?.response?.data?.error || 'Koneksi ke WA Gateway gagal')
        } finally {
            setPairingLoading(false)
        }
    }

    const handleLogout = async () => {
        if (!window.confirm('Disconnect WhatsApp? Bot tidak akan bisa menerima pesan sampai scan ulang.')) return
        setLogoutLoading(true)
        setError('')
        setSuccess('')
        try {
            await apiClient.post('/wa-session/logout')
            setSuccess('Device berhasil disconnect. Scan QR untuk connect ulang.')
            setStatus(null)
            setQrTimestamp(Date.now())
            fetchStatus()
        } catch (e) {
            setError(e?.response?.data?.error || 'Logout gagal')
        } finally {
            setLogoutLoading(false)
        }
    }

    const isConnected = status?.connected && status?.logged_in
    const isPending = status?.connected && !status?.logged_in

    // Build QR URL via proxy endpoint with auth via axios (use img tag workaround: embed as data URL)
    const qrSrc = `${baseURL}/api/v1/wa-session/qr?_t=${qrTimestamp}`

    return (
        <MainCard>
            <Stack direction='row' alignItems='center' justifyContent='space-between' mb={3}>
                <Stack direction='row' alignItems='center' gap={1}>
                    <IconDeviceMobile size={28} />
                    <Typography variant='h3'>WhatsApp Session</Typography>
                </Stack>
                <Tooltip title='Refresh status'>
                    <Button variant='outlined' size='small' startIcon={<IconRefresh size={16} />} onClick={fetchStatus}>
                        Refresh
                    </Button>
                </Tooltip>
            </Stack>

            {/* Status Card */}
            <Card variant='outlined' sx={{ mb: 3 }}>
                <CardContent>
                    <Stack direction='row' alignItems='center' gap={2}>
                        {loading ? (
                            <CircularProgress size={20} />
                        ) : (
                            <Chip
                                icon={isConnected ? <IconPlugConnected size={16} /> : <IconPlugConnectedX size={16} />}
                                label={isConnected ? 'Connected' : isPending ? 'Menunggu Scan QR' : 'Offline / Belum Terhubung'}
                                color={isConnected ? 'success' : isPending ? 'warning' : 'error'}
                                size='medium'
                            />
                        )}
                        {isConnected && status?.phone && (
                            <Typography variant='body2' color='text.secondary'>
                                +{status.phone}
                            </Typography>
                        )}
                    </Stack>

                    {isConnected && (
                        <Box mt={2}>
                            <Button
                                variant='outlined'
                                color='error'
                                size='small'
                                startIcon={logoutLoading ? <CircularProgress size={14} /> : <IconPlugConnectedX size={16} />}
                                onClick={handleLogout}
                                disabled={logoutLoading}
                            >
                                Disconnect Device
                            </Button>
                        </Box>
                    )}
                </CardContent>
            </Card>

            {error && <Alert severity='error' sx={{ mb: 2 }} onClose={() => setError('')}>{error}</Alert>}
            {success && <Alert severity='success' sx={{ mb: 2 }} onClose={() => setSuccess('')}>{success}</Alert>}

            {/* Show QR or pairing options when not connected */}
            {!isConnected && (
                <Stack direction={{ xs: 'column', md: 'row' }} gap={3}>
                    {/* QR Code */}
                    <Card variant='outlined' sx={{ flex: 1 }}>
                        <CardContent>
                            <Stack direction='row' alignItems='center' gap={1} mb={2}>
                                <IconQrcode size={20} />
                                <Typography variant='h5'>Scan QR Code</Typography>
                            </Stack>
                            <Typography variant='body2' color='text.secondary' mb={2}>
                                Buka WhatsApp → Linked Devices → Link a Device → Scan QR
                            </Typography>
                            <QRImage src={qrSrc} />
                            <Typography variant='caption' color='text.secondary' display='block' mt={1}>
                                QR refresh otomatis setiap 30 detik
                            </Typography>
                            <Button
                                size='small'
                                startIcon={<IconRefresh size={14} />}
                                onClick={() => setQrTimestamp(Date.now())}
                                sx={{ mt: 1 }}
                            >
                                Refresh QR
                            </Button>
                        </CardContent>
                    </Card>

                    <Divider orientation='vertical' flexItem sx={{ display: { xs: 'none', md: 'block' } }} />

                    {/* Pair by Phone */}
                    <Card variant='outlined' sx={{ flex: 1 }}>
                        <CardContent>
                            <Stack direction='row' alignItems='center' gap={1} mb={2}>
                                <IconPhone size={20} />
                                <Typography variant='h5'>Pair by Phone Number</Typography>
                            </Stack>
                            <Typography variant='body2' color='text.secondary' mb={2}>
                                Masukkan nomor WA yang ingin di-link (format: 628xxx atau 08xxx)
                            </Typography>
                            <TextField
                                label='Nomor WhatsApp'
                                placeholder='628123456789'
                                value={phone}
                                onChange={(e) => setPhone(e.target.value)}
                                fullWidth
                                size='small'
                                sx={{ mb: 2 }}
                                onKeyDown={(e) => e.key === 'Enter' && handlePairPhone()}
                            />
                            <Button
                                variant='contained'
                                onClick={handlePairPhone}
                                disabled={pairingLoading || !phone}
                                startIcon={pairingLoading ? <CircularProgress size={16} /> : <IconPhone size={16} />}
                                fullWidth
                            >
                                Dapatkan Kode Pairing
                            </Button>

                            {pairingCode && (
                                <Box mt={2} p={2} sx={{ bgcolor: 'success.lighter', borderRadius: 1, textAlign: 'center' }}>
                                    <Typography variant='caption' color='success.dark' display='block'>
                                        Kode Pairing (masukkan di WhatsApp → Linked Devices → Link with phone number)
                                    </Typography>
                                    <Typography variant='h3' color='success.dark' fontFamily='monospace' letterSpacing={4} mt={1}>
                                        {pairingCode}
                                    </Typography>
                                </Box>
                            )}

                            <Typography variant='caption' color='text.secondary' display='block' mt={2}>
                                Di WhatsApp: Settings → Linked Devices → Link a Device → Link with phone number
                            </Typography>
                        </CardContent>
                    </Card>
                </Stack>
            )}
        </MainCard>
    )
}

// QR image needs auth — fetch via axios and render as blob URL
function QRImage({ src }) {
    const [imgSrc, setImgSrc] = useState(null)

    useEffect(() => {
        let cancelled = false
        apiClient.get('/wa-session/qr', { responseType: 'blob' })
            .then((res) => {
                if (!cancelled) setImgSrc(URL.createObjectURL(res.data))
            })
            .catch(() => {
                if (!cancelled) setImgSrc(null)
            })
        return () => { cancelled = true }
    }, [src, apiClient])

    if (!imgSrc) return (
        <Box sx={{ width: 220, height: 220, border: '1px dashed', borderColor: 'divider', borderRadius: 1,
            display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <Typography variant='caption' color='text.secondary'>QR tidak tersedia</Typography>
        </Box>
    )

    return (
        <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 1, p: 1, display: 'inline-block', background: '#fff' }}>
            <img src={imgSrc} alt='WhatsApp QR Code' style={{ width: 220, height: 220, display: 'block' }} />
        </Box>
    )
}
