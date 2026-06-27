import { useEffect, useState, useCallback } from 'react'
import {
    Box,
    Button,
    Chip,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Divider,
    FormControl,
    IconButton,
    InputLabel,
    MenuItem,
    Paper,
    Select,
    Stack,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    TextField,
    Tooltip,
    Typography
} from '@mui/material'
import {
    IconPlus,
    IconTrash,
    IconRefresh,
    IconQrcode,
    IconPhone,
    IconPlugConnectedX,
    IconBrandWhatsapp,
    IconCheck,
    IconEye,
    IconExternalLink
} from '@tabler/icons-react'
import { useTheme } from '@mui/material/styles'
import { useNavigate } from 'react-router-dom'
import apiClient from '@/api/client'

const API = '/wa-session'

const emptyForm = { name: '', chatflow_id: '', human_contact: '', allow_phones: '', disable_upload: false }

export default function WASession() {
    const theme = useTheme()
    const navigate = useNavigate()
    const [sessions, setSessions] = useState([])
    const [chatflows, setChatflows] = useState([])
    const [loading, setLoading] = useState(false)
    const [error, setError] = useState('')
    const [addOpen, setAddOpen] = useState(false)
    const [form, setForm] = useState(emptyForm)
    const [saving, setSaving] = useState(false)
    const [saveError, setSaveError] = useState('')
    const [viewSession, setViewSession] = useState(null)
    const [connectOpen, setConnectOpen] = useState(false)
    const [activeSession, setActiveSession] = useState(null)
    const [qrImg, setQrImg] = useState(null)
    const [qrLoading, setQrLoading] = useState(false)
    const [phone, setPhone] = useState('')
    const [pairingCode, setPairingCode] = useState('')
    const [pairingLoading, setPairingLoading] = useState(false)
    const [pairingError, setPairingError] = useState('')
    const [logoutBusy, setLogoutBusy] = useState({})

    const loadChatflows = useCallback(async () => {
        try {
            const res = await apiClient.get('/chatflows')
            setChatflows(res.data || [])
        } catch { /* non-fatal */ }
    }, [])

    const load = useCallback(async () => {
        setLoading(true)
        setError('')
        try {
            const res = await apiClient.get(`${API}/sessions`)
            setSessions(res.data || [])
        } catch (e) {
            setError(e?.response?.data?.error || e.message)
        } finally {
            setLoading(false)
        }
    }, [])

    useEffect(() => {
        load()
        loadChatflows()
        const t = setInterval(load, 8000)
        return () => clearInterval(t)
    }, [load, loadChatflows])

    const getChatflow = (id) => chatflows.find((c) => c.id === id)
    const getChatflowName = (id) => getChatflow(id)?.name || (id ? '— Belum Terhubung' : '—')

    const handleAdd = async () => {
        setSaving(true)
        setSaveError('')
        try {
            await apiClient.post(`${API}/sessions`, form)
            setAddOpen(false)
            setForm(emptyForm)
            await load()
        } catch (e) {
            setSaveError(e?.response?.data?.error || e.message)
        } finally {
            setSaving(false)
        }
    }

    const handleDelete = async (id, name) => {
        if (!window.confirm(`Hapus sesi "${name}"? Bot akan disconnect.`)) return
        try {
            await apiClient.delete(`${API}/sessions/${id}`)
            await load()
        } catch (e) {
            alert('Gagal hapus: ' + (e?.response?.data?.error || e.message))
        }
    }

    const handleLogout = async (id) => {
        setLogoutBusy((p) => ({ ...p, [id]: true }))
        try {
            await apiClient.post(`${API}/sessions/${id}/logout`)
            await load()
        } catch (e) {
            alert('Gagal disconnect: ' + (e?.response?.data?.error || e.message))
        } finally {
            setLogoutBusy((p) => ({ ...p, [id]: false }))
        }
    }

    const openConnect = async (session) => {
        setActiveSession(session)
        setQrImg(null)
        setPhone('')
        setPairingCode('')
        setPairingError('')
        setConnectOpen(true)
        try { await apiClient.post(`${API}/sessions/${session.id}/connect`) } catch { /* already connecting */ }
        await refreshQR(session.id)
    }

    const refreshQR = async (id) => {
        setQrLoading(true)
        try {
            const res = await apiClient.get(`${API}/sessions/${id}/qr`, { responseType: 'blob' })
            setQrImg(URL.createObjectURL(res.data))
        } catch { setQrImg(null) }
        finally { setQrLoading(false) }
    }

    const handlePairPhone = async () => {
        if (!activeSession) return
        const cleaned = phone.replace(/\D/g, '').replace(/^0/, '62').replace(/^\+/, '')
        if (!cleaned) { setPairingError('Masukkan nomor telepon'); return }
        setPairingError('')
        setPairingCode('')
        setPairingLoading(true)
        try {
            const res = await apiClient.post(`${API}/sessions/${activeSession.id}/pair-phone?phone=${cleaned}`)
            if (res.data?.pairing_code) setPairingCode(res.data.pairing_code)
            else setPairingError(res.data?.error || 'Gagal mendapat kode pairing')
        } catch (e) {
            setPairingError(e?.response?.data?.error || 'Gagal')
        } finally { setPairingLoading(false) }
    }

    const statusColor = (s) => s === 'connected' ? 'success' : s === 'qr_pending' ? 'warning' : 'default'
    const statusLabel = (s) => s === 'connected' ? 'Connected' : s === 'qr_pending' ? 'Pending QR' : 'Offline'

    const chatflowType = (id) => {
        const cf = getChatflow(id)
        if (!cf) return null
        return cf.type === 'MULTIAGENT' || cf.name?.toLowerCase().includes('agent') ? 'agentflows' : 'chatflows'
    }

    return (
        <Box sx={{ p: 3 }}>
            <Stack direction='row' alignItems='center' justifyContent='space-between' mb={3}>
                <Stack direction='row' alignItems='center' gap={1}>
                    <IconBrandWhatsapp size={28} color={theme.palette.primary.main} />
                    <Typography variant='h4'>WA Bots</Typography>
                </Stack>
                <Stack direction='row' gap={1}>
                    <Button variant='outlined' startIcon={<IconRefresh size={16} />} onClick={load} disabled={loading}>Refresh</Button>
                    <Button variant='contained' startIcon={<IconPlus size={16} />} onClick={() => { setForm(emptyForm); setSaveError(''); setAddOpen(true) }}>
                        Tambah Sesi
                    </Button>
                </Stack>
            </Stack>

            {error && (
                <Paper sx={{ p: 2, mb: 2, bgcolor: 'error.light', color: 'error.contrastText' }}>
                    <Typography>{error}</Typography>
                </Paper>
            )}

            <TableContainer component={Paper}>
                <Table>
                    <TableHead>
                        <TableRow>
                            <TableCell>Nama Bot</TableCell>
                            <TableCell>Nomor WA</TableCell>
                            <TableCell>Agentflow</TableCell>
                            <TableCell>Status</TableCell>
                            <TableCell align='right'>Aksi</TableCell>
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {sessions.length === 0 && (
                            <TableRow>
                                <TableCell colSpan={5} align='center' sx={{ py: 4, color: 'text.secondary' }}>
                                    {loading ? 'Memuat...' : 'Belum ada sesi WA. Klik "Tambah Sesi" untuk mulai.'}
                                </TableCell>
                            </TableRow>
                        )}
                        {sessions.map((s) => (
                            <TableRow key={s.id} hover>
                                <TableCell>
                                    <Typography fontWeight={600}>{s.name}</Typography>
                                    {s.human_contact && (
                                        <Typography variant='caption' color='text.secondary'>Admin: {s.human_contact}</Typography>
                                    )}
                                </TableCell>
                                <TableCell>
                                    <Typography variant='body2' fontFamily='monospace'>
                                        {s.phone ? '+' + s.phone : '—'}
                                    </Typography>
                                </TableCell>
                                <TableCell>
                                    <Typography variant='body2'>{getChatflowName(s.chatflow_id)}</Typography>
                                </TableCell>
                                <TableCell>
                                    <Chip label={statusLabel(s.status)} color={statusColor(s.status)} size='small' />
                                </TableCell>
                                <TableCell align='right'>
                                    <Stack direction='row' justifyContent='flex-end' gap={0.5}>
                                        <Tooltip title='Lihat detail'>
                                            <IconButton size='small' onClick={() => setViewSession(s)}>
                                                <IconEye size={16} />
                                            </IconButton>
                                        </Tooltip>
                                        {s.status !== 'connected' && (
                                            <Tooltip title='Connect / Scan QR'>
                                                <IconButton size='small' color='primary' onClick={() => openConnect(s)}>
                                                    <IconQrcode size={16} />
                                                </IconButton>
                                            </Tooltip>
                                        )}
                                        {s.status === 'connected' && (
                                            <Tooltip title='Disconnect'>
                                                <IconButton size='small' color='warning' onClick={() => handleLogout(s.id)} disabled={!!logoutBusy[s.id]}>
                                                    {logoutBusy[s.id] ? <CircularProgress size={14} /> : <IconPlugConnectedX size={16} />}
                                                </IconButton>
                                            </Tooltip>
                                        )}
                                        <Tooltip title='Hapus sesi'>
                                            <IconButton size='small' color='error' onClick={() => handleDelete(s.id, s.name)}>
                                                <IconTrash size={16} />
                                            </IconButton>
                                        </Tooltip>
                                    </Stack>
                                </TableCell>
                            </TableRow>
                        ))}
                    </TableBody>
                </Table>
            </TableContainer>

            {/* Add Session Dialog */}
            <Dialog open={addOpen} onClose={() => setAddOpen(false)} fullWidth maxWidth='sm'>
                <DialogTitle>Tambah Sesi WhatsApp</DialogTitle>
                <DialogContent>
                    <Stack gap={2} mt={1}>
                        {saveError && (
                            <Paper sx={{ p: 1.5, bgcolor: 'error.light', color: 'error.contrastText' }}>
                                <Typography variant='body2'>{saveError}</Typography>
                            </Paper>
                        )}
                        <TextField label='Nama Bot' value={form.name}
                            onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                            placeholder='Customer Service WA' fullWidth required />

                        <FormControl fullWidth required>
                            <InputLabel>Agentflow / Chatflow</InputLabel>
                            <Select value={form.chatflow_id} label='Agentflow / Chatflow'
                                displayEmpty
                                onChange={(e) => setForm((f) => ({ ...f, chatflow_id: e.target.value }))}>
                                <MenuItem disabled value=''>
                                    <Typography color='text.secondary'>{chatflows.length === 0 ? 'Memuat daftar...' : '— Pilih Agentflow —'}</Typography>
                                </MenuItem>
                                {chatflows.map((cf) => (
                                    <MenuItem key={cf.id} value={cf.id}>
                                        <Stack>
                                            <Typography variant='body2'>{cf.name}</Typography>
                                            <Typography variant='caption' color='text.secondary' fontFamily='monospace'>{cf.id}</Typography>
                                        </Stack>
                                    </MenuItem>
                                ))}
                            </Select>
                        </FormControl>

                        <TextField label='Kontak Admin (opsional)' value={form.human_contact}
                            onChange={(e) => setForm((f) => ({ ...f, human_contact: e.target.value }))}
                            placeholder='@username atau nomor WA' fullWidth
                            helperText='Ditampilkan ke user saat terjadi error' />
                        <TextField label='Batasi Nomor (opsional)' value={form.allow_phones}
                            onChange={(e) => setForm((f) => ({ ...f, allow_phones: e.target.value }))}
                            placeholder='628123456789,628987654321' fullWidth
                            helperText='Kosongkan = semua. Isi nomor dipisah koma untuk bot privat.' />
                    </Stack>
                </DialogContent>
                <DialogActions sx={{ px: 3, pb: 2 }}>
                    <Button onClick={() => setAddOpen(false)} disabled={saving}>Batal</Button>
                    <Button variant='contained' onClick={handleAdd} disabled={saving || !form.name || !form.chatflow_id}>
                        {saving ? 'Menyimpan...' : 'Simpan'}
                    </Button>
                </DialogActions>
            </Dialog>

            {/* View Detail Dialog */}
            <Dialog open={!!viewSession} onClose={() => setViewSession(null)} fullWidth maxWidth='sm'>
                <DialogTitle>Detail Bot — {viewSession?.name}</DialogTitle>
                <DialogContent>
                    {viewSession && (
                        <Stack gap={2} mt={1}>
                            <DetailRow label='Session ID'>
                                <Typography variant='body2' fontFamily='monospace' fontSize={12}>{viewSession.id}</Typography>
                            </DetailRow>
                            <DetailRow label='Nomor WA'>
                                <Typography variant='body2' fontFamily='monospace'>
                                    {viewSession.phone ? '+' + viewSession.phone : '— (belum terhubung)'}
                                </Typography>
                            </DetailRow>
                            <DetailRow label='Status'>
                                <Chip label={statusLabel(viewSession.status)} color={statusColor(viewSession.status)} size='small' />
                            </DetailRow>
                            <DetailRow label='Agentflow'>
                                <Stack direction='row' alignItems='center' gap={1}>
                                    <Typography variant='body2'>{getChatflowName(viewSession.chatflow_id)}</Typography>
                                    <Tooltip title='Buka Agentflow'>
                                        <IconButton size='small' onClick={() => {
                                            const type = chatflowType(viewSession.chatflow_id) || 'agentflows'
                                            navigate(`/${type}/${viewSession.chatflow_id}`)
                                            setViewSession(null)
                                        }}>
                                            <IconExternalLink size={14} />
                                        </IconButton>
                                    </Tooltip>
                                </Stack>
                                <Typography variant='caption' color='text.secondary' fontFamily='monospace'>{viewSession.chatflow_id}</Typography>
                            </DetailRow>
                            {viewSession.human_contact && (
                                <DetailRow label='Kontak Admin'>
                                    <Typography variant='body2'>{viewSession.human_contact}</Typography>
                                </DetailRow>
                            )}
                            {viewSession.allow_phones && (
                                <DetailRow label='Batasi Nomor'>
                                    <Typography variant='body2' fontFamily='monospace'>{viewSession.allow_phones}</Typography>
                                </DetailRow>
                            )}
                        </Stack>
                    )}
                </DialogContent>
                <DialogActions sx={{ px: 3, pb: 2 }}>
                    {viewSession?.status !== 'connected' && (
                        <Button startIcon={<IconQrcode size={16} />} onClick={() => { openConnect(viewSession); setViewSession(null) }}>
                            Connect
                        </Button>
                    )}
                    <Button variant='contained' onClick={() => setViewSession(null)}>Tutup</Button>
                </DialogActions>
            </Dialog>

            {/* Connect / QR Dialog */}
            <Dialog open={connectOpen} onClose={() => setConnectOpen(false)} fullWidth maxWidth='sm'>
                <DialogTitle>Hubungkan WhatsApp — {activeSession?.name}</DialogTitle>
                <DialogContent>
                    <Stack direction={{ xs: 'column', sm: 'row' }} gap={3} mt={1}>
                        <Box flex={1} textAlign='center'>
                            <Stack direction='row' alignItems='center' gap={1} mb={1.5}>
                                <IconQrcode size={18} />
                                <Typography variant='subtitle2'>Scan QR</Typography>
                            </Stack>
                            <Typography variant='caption' color='text.secondary' display='block' mb={1.5}>
                                WA → Linked Devices → Link a Device → Scan QR
                            </Typography>
                            {qrLoading ? (
                                <Box sx={{ width: 200, height: 200, display: 'flex', alignItems: 'center', justifyContent: 'center', mx: 'auto' }}>
                                    <CircularProgress />
                                </Box>
                            ) : qrImg ? (
                                <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 1, p: 1, display: 'inline-block', bgcolor: '#fff' }}>
                                    <img src={qrImg} alt='QR' style={{ width: 200, height: 200, display: 'block' }} />
                                </Box>
                            ) : (
                                <Box sx={{ width: 200, height: 200, border: '1px dashed', borderColor: 'divider', borderRadius: 1,
                                    display: 'flex', alignItems: 'center', justifyContent: 'center', mx: 'auto' }}>
                                    <Typography variant='caption' color='text.secondary' textAlign='center'>QR belum tersedia.<br/>Klik Refresh.</Typography>
                                </Box>
                            )}
                            <Button size='small' startIcon={<IconRefresh size={14} />} sx={{ mt: 1 }}
                                onClick={() => activeSession && refreshQR(activeSession.id)}>
                                Refresh QR
                            </Button>
                        </Box>

                        <Divider orientation='vertical' flexItem sx={{ display: { xs: 'none', sm: 'block' } }} />

                        <Box flex={1}>
                            <Stack direction='row' alignItems='center' gap={1} mb={1.5}>
                                <IconPhone size={18} />
                                <Typography variant='subtitle2'>Pair via Nomor</Typography>
                            </Stack>
                            <Typography variant='caption' color='text.secondary' display='block' mb={1.5}>
                                WA → Linked Devices → Link a Device → Link with phone number
                            </Typography>
                            <TextField label='Nomor WA' placeholder='628123456789' value={phone}
                                onChange={(e) => setPhone(e.target.value)} fullWidth size='small' sx={{ mb: 1.5 }}
                                onKeyDown={(e) => e.key === 'Enter' && handlePairPhone()} />
                            <Button variant='contained' fullWidth onClick={handlePairPhone}
                                disabled={pairingLoading || !phone}
                                startIcon={pairingLoading ? <CircularProgress size={14} /> : <IconPhone size={14} />}>
                                Dapatkan Kode
                            </Button>
                            {pairingError && <Typography variant='caption' color='error' display='block' mt={1}>{pairingError}</Typography>}
                            {pairingCode && (
                                <Box mt={2} p={1.5} sx={{ bgcolor: 'success.lighter', borderRadius: 1, textAlign: 'center' }}>
                                    <Typography variant='caption' color='success.dark' display='block'>Kode Pairing</Typography>
                                    <Typography variant='h4' color='success.dark' fontFamily='monospace' letterSpacing={4} mt={0.5}>{pairingCode}</Typography>
                                    <Stack direction='row' alignItems='center' justifyContent='center' gap={0.5} mt={0.5}>
                                        <IconCheck size={14} />
                                        <Typography variant='caption' color='success.dark'>Masukkan di WA</Typography>
                                    </Stack>
                                </Box>
                            )}
                        </Box>
                    </Stack>
                </DialogContent>
                <DialogActions sx={{ px: 3, pb: 2 }}>
                    <Button onClick={() => setConnectOpen(false)}>Tutup</Button>
                </DialogActions>
            </Dialog>
        </Box>
    )
}

function DetailRow({ label, children }) {
    return (
        <Box>
            <Typography variant='caption' color='text.secondary' display='block' mb={0.5}>{label}</Typography>
            {children}
        </Box>
    )
}
