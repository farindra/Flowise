import { useEffect, useState, useCallback } from 'react'
import {
    Box,
    Button,
    Chip,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
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
import { IconPlus, IconTrash, IconRefresh, IconBrandTelegram, IconCheck, IconEye, IconExternalLink } from '@tabler/icons-react'
import { useTheme } from '@mui/material/styles'
import { useNavigate } from 'react-router-dom'
import apiClient from '@/api/client'

const API = '/api/v1/telegram-session'

async function apiFetch(path, options = {}) {
    const res = await fetch(API + path, {
        headers: { 'Content-Type': 'application/json', ...options.headers },
        ...options
    })
    if (!res.ok) {
        const err = await res.json().catch(() => ({ error: res.statusText }))
        throw new Error(err.error || res.statusText)
    }
    return res.json()
}

const emptyForm = { name: '', token: '', chatflow_id: '', allow_user_ids: '', disable_upload: false, human_contact: '' }

export default function TelegramSession() {
    const theme = useTheme()
    const navigate = useNavigate()
    const [bots, setBots] = useState([])
    const [chatflows, setChatflows] = useState([])
    const [loading, setLoading] = useState(false)
    const [error, setError] = useState('')
    const [dialogOpen, setDialogOpen] = useState(false)
    const [form, setForm] = useState(emptyForm)
    const [saving, setSaving] = useState(false)
    const [saveError, setSaveError] = useState('')
    const [registering, setRegistering] = useState({})
    const [viewBot, setViewBot] = useState(null)

    const loadChatflows = useCallback(async () => {
        try {
            const res = await apiClient.get('/chatflows')
            setChatflows(res.data || [])
        } catch { /* non-fatal */ }
    }, [])

    const load = async () => {
        setLoading(true)
        setError('')
        try {
            const data = await apiFetch('/bots')
            setBots(data || [])
        } catch (e) {
            setError(e.message)
        } finally {
            setLoading(false)
        }
    }

    useEffect(() => {
        load()
        loadChatflows()
    }, [])

    const getChatflow = (id) => chatflows.find((c) => c.id === id)
    const getChatflowName = (id) => getChatflow(id)?.name || (id ? '— Belum Terhubung' : '—')

    const chatflowType = (id) => {
        const cf = getChatflow(id)
        if (!cf) return 'agentflows'
        return cf.type === 'MULTIAGENT' || cf.name?.toLowerCase().includes('agent') ? 'agentflows' : 'chatflows'
    }

    const handleAdd = async () => {
        setSaving(true)
        setSaveError('')
        try {
            await apiFetch('/bots', { method: 'POST', body: JSON.stringify(form) })
            setDialogOpen(false)
            setForm(emptyForm)
            await load()
        } catch (e) {
            setSaveError(e.message)
        } finally {
            setSaving(false)
        }
    }

    const handleDelete = async (id, name) => {
        if (!window.confirm(`Hapus bot "${name}"?`)) return
        try {
            await apiFetch(`/bots/${id}`, { method: 'DELETE' })
            await load()
        } catch (e) {
            alert('Gagal hapus: ' + e.message)
        }
    }

    const handleRegister = async (id) => {
        setRegistering((prev) => ({ ...prev, [id]: true }))
        try {
            await apiFetch(`/bots/${id}/register`, { method: 'POST' })
        } catch (e) {
            alert('Gagal register webhook: ' + e.message)
        } finally {
            setTimeout(() => setRegistering((prev) => ({ ...prev, [id]: false })), 2000)
        }
    }

    return (
        <Box sx={{ p: 3 }}>
            <Stack direction='row' alignItems='center' justifyContent='space-between' mb={3}>
                <Stack direction='row' alignItems='center' gap={1}>
                    <IconBrandTelegram size={28} color={theme.palette.primary.main} />
                    <Typography variant='h4'>Telegram Bots</Typography>
                </Stack>
                <Stack direction='row' gap={1}>
                    <Button variant='outlined' startIcon={<IconRefresh size={16} />} onClick={load} disabled={loading}>
                        Refresh
                    </Button>
                    <Button variant='contained' startIcon={<IconPlus size={16} />} onClick={() => { setForm(emptyForm); setSaveError(''); setDialogOpen(true) }}>
                        Tambah Bot
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
                            <TableCell>Token</TableCell>
                            <TableCell>Agentflow</TableCell>
                            <TableCell>Status</TableCell>
                            <TableCell align='right'>Aksi</TableCell>
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {bots.length === 0 && (
                            <TableRow>
                                <TableCell colSpan={5} align='center' sx={{ py: 4, color: 'text.secondary' }}>
                                    {loading ? 'Memuat...' : 'Belum ada bot. Klik "Tambah Bot" untuk mulai.'}
                                </TableCell>
                            </TableRow>
                        )}
                        {bots.map((bot) => (
                            <TableRow key={bot.id} hover>
                                <TableCell>
                                    <Typography fontWeight={600}>{bot.name}</Typography>
                                    {bot.human_contact && (
                                        <Typography variant='caption' color='text.secondary'>Admin: {bot.human_contact}</Typography>
                                    )}
                                </TableCell>
                                <TableCell>
                                    <Typography variant='body2' fontFamily='monospace' fontSize={12}>
                                        {bot.token_masked}
                                    </Typography>
                                </TableCell>
                                <TableCell>
                                    <Typography variant='body2'>{getChatflowName(bot.chatflow_id)}</Typography>
                                </TableCell>
                                <TableCell>
                                    <Chip
                                        label={bot.active ? 'Aktif' : 'Nonaktif'}
                                        color={bot.active ? 'success' : 'default'}
                                        size='small'
                                    />
                                    {bot.disable_upload && <Chip label='No Upload' size='small' sx={{ ml: 0.5 }} />}
                                </TableCell>
                                <TableCell align='right'>
                                    <Stack direction='row' justifyContent='flex-end' gap={0.5}>
                                        <Tooltip title='Lihat detail'>
                                            <IconButton size='small' onClick={() => setViewBot(bot)}>
                                                <IconEye size={16} />
                                            </IconButton>
                                        </Tooltip>
                                        <Tooltip title='Re-register webhook'>
                                            <IconButton
                                                size='small'
                                                color={registering[bot.id] ? 'success' : 'primary'}
                                                onClick={() => handleRegister(bot.id)}
                                                disabled={!!registering[bot.id]}
                                            >
                                                {registering[bot.id] ? <IconCheck size={16} /> : <IconRefresh size={16} />}
                                            </IconButton>
                                        </Tooltip>
                                        <Tooltip title='Hapus bot'>
                                            <IconButton size='small' color='error' onClick={() => handleDelete(bot.id, bot.name)}>
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

            {/* Add Bot Dialog */}
            <Dialog open={dialogOpen} onClose={() => setDialogOpen(false)} fullWidth maxWidth='sm'>
                <DialogTitle>Tambah Telegram Bot</DialogTitle>
                <DialogContent>
                    <Stack gap={2} mt={1}>
                        {saveError && (
                            <Paper sx={{ p: 1.5, bgcolor: 'error.light', color: 'error.contrastText' }}>
                                <Typography variant='body2'>{saveError}</Typography>
                            </Paper>
                        )}
                        <TextField
                            label='Nama Bot' value={form.name}
                            onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                            placeholder='Customer Service Bot' fullWidth required
                        />
                        <TextField
                            label='Bot Token' value={form.token}
                            onChange={(e) => setForm((f) => ({ ...f, token: e.target.value }))}
                            placeholder='1234567890:AABBccDDeeFFggHH...' fullWidth required
                            helperText='Dapatkan dari @BotFather di Telegram'
                        />
                        <FormControl fullWidth required>
                            <InputLabel>Agentflow / Chatflow</InputLabel>
                            <Select value={form.chatflow_id} label='Agentflow / Chatflow'
                                displayEmpty
                                renderValue={(val) => val
                                    ? <Typography variant='body2'>{getChatflowName(val)}</Typography>
                                    : <Typography variant='body2' color='text.secondary'>— Pilih Agentflow —</Typography>
                                }
                                onChange={(e) => setForm((f) => ({ ...f, chatflow_id: e.target.value }))}>
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
                        <TextField
                            label='Kontak Admin (opsional)' value={form.human_contact}
                            onChange={(e) => setForm((f) => ({ ...f, human_contact: e.target.value }))}
                            placeholder='@username atau nomor WA' fullWidth
                            helperText='Ditampilkan ke user saat terjadi error'
                        />
                        <TextField
                            label='Batasi User ID (opsional)' value={form.allow_user_ids}
                            onChange={(e) => setForm((f) => ({ ...f, allow_user_ids: e.target.value }))}
                            placeholder='123456789,987654321' fullWidth
                            helperText='Kosongkan = semua user boleh. Isi Telegram User ID dipisah koma untuk bot privat.'
                        />
                    </Stack>
                </DialogContent>
                <DialogActions sx={{ px: 3, pb: 2 }}>
                    <Button onClick={() => setDialogOpen(false)} disabled={saving}>Batal</Button>
                    <Button variant='contained' onClick={handleAdd}
                        disabled={saving || !form.name || !form.token || !form.chatflow_id}>
                        {saving ? 'Menyimpan...' : 'Simpan'}
                    </Button>
                </DialogActions>
            </Dialog>

            {/* View Detail Dialog */}
            <Dialog open={!!viewBot} onClose={() => setViewBot(null)} fullWidth maxWidth='sm'>
                <DialogTitle>Detail Bot — {viewBot?.name}</DialogTitle>
                <DialogContent>
                    {viewBot && (
                        <Stack gap={2} mt={1}>
                            <DetailRow label='Bot ID'>
                                <Typography variant='body2' fontFamily='monospace' fontSize={12}>{viewBot.id}</Typography>
                            </DetailRow>
                            <DetailRow label='Token'>
                                <Typography variant='body2' fontFamily='monospace' fontSize={12}>{viewBot.token_masked}</Typography>
                            </DetailRow>
                            <DetailRow label='Webhook URL'>
                                <Typography variant='body2' fontSize={12} color='text.secondary' sx={{ wordBreak: 'break-all' }}>
                                    {viewBot.webhook_url || '—'}
                                </Typography>
                            </DetailRow>
                            <DetailRow label='Status'>
                                <Chip label={viewBot.active ? 'Aktif' : 'Nonaktif'} color={viewBot.active ? 'success' : 'default'} size='small' />
                            </DetailRow>
                            <DetailRow label='Agentflow'>
                                <Stack direction='row' alignItems='center' gap={1}>
                                    <Typography variant='body2'>{getChatflowName(viewBot.chatflow_id)}</Typography>
                                    <Tooltip title='Buka Agentflow'>
                                        <IconButton size='small' onClick={() => {
                                            const type = chatflowType(viewBot.chatflow_id)
                                            navigate(`/${type}/${viewBot.chatflow_id}`)
                                            setViewBot(null)
                                        }}>
                                            <IconExternalLink size={14} />
                                        </IconButton>
                                    </Tooltip>
                                </Stack>
                                <Typography variant='caption' color='text.secondary' fontFamily='monospace'>{viewBot.chatflow_id}</Typography>
                            </DetailRow>
                            {viewBot.human_contact && (
                                <DetailRow label='Kontak Admin'>
                                    <Typography variant='body2'>{viewBot.human_contact}</Typography>
                                </DetailRow>
                            )}
                            {viewBot.allow_user_ids && (
                                <DetailRow label='Batasi User ID'>
                                    <Typography variant='body2' fontFamily='monospace'>{viewBot.allow_user_ids}</Typography>
                                </DetailRow>
                            )}
                        </Stack>
                    )}
                </DialogContent>
                <DialogActions sx={{ px: 3, pb: 2 }}>
                    <Button startIcon={<IconRefresh size={16} />} onClick={() => { handleRegister(viewBot.id); setViewBot(null) }}>
                        Re-register Webhook
                    </Button>
                    <Button variant='contained' onClick={() => setViewBot(null)}>Tutup</Button>
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
