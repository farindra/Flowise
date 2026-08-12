import PropTypes from 'prop-types'
import { useEffect, useRef, useState } from 'react'
import {
    Alert,
    Box,
    Checkbox,
    Chip,
    CircularProgress,
    Dialog,
    DialogContent,
    DialogTitle,
    Divider,
    FormControl,
    FormControlLabel,
    InputLabel,
    MenuItem,
    Paper,
    Radio,
    RadioGroup,
    Select,
    Stack,
    Step,
    StepLabel,
    Stepper,
    Switch,
    TextField,
    Typography
} from '@mui/material'
import { IconPhoto, IconSend, IconTrash } from '@tabler/icons-react'
import broadcastApi from '@/api/crmbroadcast'
import AudiencePicker from './AudiencePicker'
import ThrottleForm from './ThrottleForm'

const STEPS = ['Pesan', 'Audiens', 'Pengaturan Kirim', 'Jadwal & Review']

// Variables always available; upload columns add more at runtime.
const BASE_VARS = ['nama', 'wilayah', 'tier']

const emptyForm = {
    name: '',
    sender_id: '',
    message_text: '',
    media_path: '',
    media_mime: '',
    media_mode: 'caption',
    audience: { customers: { tiers: [], wilayahs: [] } },
    throttle: {},
    include_blacklist: false,
    all_phones: false,
    dry_run: false,
    scheduled_at: null
}

const CampaignDialog = ({ open, campaign, onClose, onSaved }) => {
    const [step, setStep] = useState(0)
    const [form, setForm] = useState(emptyForm)
    const [senders, setSenders] = useState([])
    const [throttleDefaults, setThrottleDefaults] = useState({})
    const [limits, setLimits] = useState(null)
    const [mediaPreview, setMediaPreview] = useState('')
    const [saving, setSaving] = useState(false)
    const [error, setError] = useState('')
    const [testPhone, setTestPhone] = useState('')
    const [testResult, setTestResult] = useState(null)
    const [testing, setTesting] = useState(false)
    const [confirmed, setConfirmed] = useState(false)
    const [scheduleMode, setScheduleMode] = useState('now')
    const fileRef = useRef(null)

    useEffect(() => {
        if (!open) return
        setStep(0)
        setError('')
        setTestResult(null)
        setConfirmed(false)
        setMediaPreview('')
        if (campaign) {
            setForm({ ...emptyForm, ...campaign, throttle: campaign.throttle_overrides || {} })
            setScheduleMode(campaign.scheduled_at ? 'later' : 'now')
        } else {
            setForm(emptyForm)
            setScheduleMode('now')
        }
        broadcastApi
            .getSenders()
            .then((r) => setSenders(r.data || []))
            .catch((e) => setError(e.response?.data?.error || e.message))
        broadcastApi
            .getThrottleDefaults()
            .then((r) => {
                setThrottleDefaults(r.data?.defaults || {})
                setLimits(r.data?.limits || null)
            })
            .catch(() => {})
    }, [open, campaign])

    const patch = (p) => setForm((f) => ({ ...f, ...p }))

    const uploadVars = (form.audience?.upload || []).reduce((acc, row) => {
        Object.keys(row.vars || {}).forEach((k) => {
            if (!acc.includes(k)) acc.push(k)
        })
        return acc
    }, [])
    const allVars = [...BASE_VARS, ...uploadVars]

    const insertVar = (v) => patch({ message_text: `${form.message_text}{{${v}}}` })

    const handleMedia = async (e) => {
        const file = e.target.files?.[0]
        e.target.value = ''
        if (!file) return
        setError('')
        try {
            const fd = new FormData()
            fd.append('file', file)
            const { data } = await broadcastApi.uploadMedia(fd)
            patch({ media_path: data.media_path, media_mime: data.media_mime })
            // Preview from the local File — an authenticated GET would need
            // headers an <img src> cannot send.
            setMediaPreview(URL.createObjectURL(file))
        } catch (err) {
            setError(err.response?.data?.error || err.message)
        }
    }

    const clearMedia = () => {
        patch({ media_path: '', media_mime: '' })
        setMediaPreview('')
    }

    const save = async () => {
        setSaving(true)
        setError('')
        try {
            const payload = {
                ...form,
                scheduled_at: scheduleMode === 'later' && form.scheduled_at ? new Date(form.scheduled_at).toISOString() : null
            }
            let id = campaign?.id
            if (id) {
                await broadcastApi.updateBroadcast(id, payload)
            } else {
                const { data } = await broadcastApi.createBroadcast(payload)
                id = data.id
            }
            onSaved(id)
            return id
        } catch (err) {
            setError(err.response?.data?.error || err.message)
            return null
        } finally {
            setSaving(false)
        }
    }

    const runTest = async () => {
        setTesting(true)
        setTestResult(null)
        setError('')
        try {
            // Test-send needs a persisted campaign to read media/message from.
            const id = await save()
            if (!id) return
            const { data } = await broadcastApi.testSend(id, { phone: testPhone })
            setTestResult({ ok: true, preview: data.preview })
        } catch (err) {
            setTestResult({ ok: false, error: err.response?.data?.error || err.message })
        } finally {
            setTesting(false)
        }
    }

    const captionLen = form.message_text.length
    const captionTooLong = form.media_path && form.media_mode === 'caption' && captionLen > 1000

    return (
        <Dialog open={open} onClose={onClose} fullWidth maxWidth='md'>
            <DialogTitle>{campaign ? 'Edit Broadcast' : 'Buat Broadcast'}</DialogTitle>
            <DialogContent>
                <Stepper activeStep={step} sx={{ mb: 3, mt: 1 }}>
                    {STEPS.map((label) => (
                        <Step key={label}>
                            <StepLabel>{label}</StepLabel>
                        </Step>
                    ))}
                </Stepper>

                {error && (
                    <Alert severity='error' sx={{ mb: 2 }}>
                        {error}
                    </Alert>
                )}

                {/* Step 1 — Pesan */}
                {step === 0 && (
                    <Stack spacing={2}>
                        <TextField
                            size='small'
                            fullWidth
                            label='Nama campaign'
                            value={form.name}
                            onChange={(e) => patch({ name: e.target.value })}
                        />
                        <FormControl size='small' fullWidth>
                            <InputLabel>Kirim dari</InputLabel>
                            <Select value={form.sender_id} label='Kirim dari' onChange={(e) => patch({ sender_id: e.target.value })}>
                                {senders.map((s) => (
                                    <MenuItem key={s.id} value={s.id} disabled={s.status !== 'connected'}>
                                        {s.label} {s.phone ? `(${s.phone})` : ''}
                                        {s.status !== 'connected' && ' — sesi tidak terhubung'}
                                    </MenuItem>
                                ))}
                            </Select>
                        </FormControl>

                        <TextField
                            size='small'
                            fullWidth
                            multiline
                            rows={6}
                            label='Isi pesan'
                            value={form.message_text}
                            onChange={(e) => patch({ message_text: e.target.value })}
                            helperText={`${captionLen} karakter · *tebal* _miring_ didukung WhatsApp`}
                        />
                        <Stack direction='row' spacing={1} flexWrap='wrap' useFlexGap>
                            <Typography variant='caption' color='text.secondary' sx={{ alignSelf: 'center' }}>
                                Sisipkan:
                            </Typography>
                            {allVars.map((v) => (
                                <Chip key={v} size='small' variant='outlined' label={`{{${v}}}`} clickable onClick={() => insertVar(v)} />
                            ))}
                        </Stack>

                        <Divider />
                        <Stack direction='row' spacing={1} alignItems='center'>
                            <Chip
                                size='small'
                                variant='outlined'
                                label={form.media_path ? 'Ganti Gambar' : 'Tambah Gambar'}
                                clickable
                                icon={<IconPhoto size={14} />}
                                onClick={() => fileRef.current?.click()}
                            />
                            {form.media_path && (
                                <Chip
                                    size='small'
                                    color='error'
                                    variant='outlined'
                                    label='Hapus'
                                    clickable
                                    icon={<IconTrash size={14} />}
                                    onClick={clearMedia}
                                />
                            )}
                            <input ref={fileRef} type='file' accept='image/jpeg,image/png,image/webp' hidden onChange={handleMedia} />
                        </Stack>
                        {mediaPreview && <Box component='img' src={mediaPreview} sx={{ maxWidth: 240, borderRadius: 1 }} />}
                        {form.media_path && (
                            <FormControl size='small' fullWidth>
                                <InputLabel>Mode gambar</InputLabel>
                                <Select value={form.media_mode} label='Mode gambar' onChange={(e) => patch({ media_mode: e.target.value })}>
                                    <MenuItem value='caption'>Teks sebagai caption gambar</MenuItem>
                                    <MenuItem value='separate'>Gambar dulu, teks pesan terpisah</MenuItem>
                                </Select>
                            </FormControl>
                        )}
                        {captionTooLong && (
                            <Alert severity='warning'>
                                Caption di atas 1000 karakter — WhatsApp membatasi sekitar 1024. Disarankan pakai mode &quot;gambar dulu,
                                teks terpisah&quot;.
                            </Alert>
                        )}
                    </Stack>
                )}

                {/* Step 2 — Audiens */}
                {step === 1 && (
                    <AudiencePicker
                        value={form.audience}
                        onChange={(a) => patch({ audience: a })}
                        includeBlacklist={form.include_blacklist}
                        onIncludeBlacklistChange={(v) => patch({ include_blacklist: v })}
                        allPhones={form.all_phones}
                        onAllPhonesChange={(v) => patch({ all_phones: v })}
                    />
                )}

                {/* Step 3 — Throttle */}
                {step === 2 && (
                    <Stack spacing={2}>
                        <ThrottleForm
                            value={form.throttle}
                            onChange={(t) => patch({ throttle: t })}
                            defaults={throttleDefaults}
                            limits={limits}
                        />
                        <Divider />
                        <FormControlLabel
                            control={<Switch checked={form.dry_run} onChange={(e) => patch({ dry_run: e.target.checked })} />}
                            label='Mode uji coba (dry run)'
                        />
                        {form.dry_run && (
                            <Alert severity='info'>
                                Dry run menjalankan seluruh alur — audiens, pacing, cooldown, batas harian — tapi tidak benar-benar mengirim
                                pesan WhatsApp.
                            </Alert>
                        )}
                    </Stack>
                )}

                {/* Step 4 — Jadwal & review */}
                {step === 3 && (
                    <Stack spacing={2}>
                        <RadioGroup value={scheduleMode} onChange={(e) => setScheduleMode(e.target.value)}>
                            <FormControlLabel value='now' control={<Radio />} label='Kirim sekarang' />
                            <FormControlLabel value='later' control={<Radio />} label='Jadwalkan' />
                        </RadioGroup>
                        {scheduleMode === 'later' && (
                            <TextField
                                size='small'
                                type='datetime-local'
                                label='Waktu kirim'
                                InputLabelProps={{ shrink: true }}
                                value={form.scheduled_at ? String(form.scheduled_at).slice(0, 16) : ''}
                                onChange={(e) => patch({ scheduled_at: e.target.value })}
                            />
                        )}

                        <Divider />
                        <Typography variant='subtitle2'>Kirim tes dulu</Typography>
                        <Stack direction='row' spacing={1}>
                            <TextField
                                size='small'
                                placeholder='08xxxxxxxxxx'
                                value={testPhone}
                                onChange={(e) => setTestPhone(e.target.value)}
                                sx={{ flex: 1 }}
                            />
                            <Chip
                                size='small'
                                color='primary'
                                label='Kirim Tes'
                                clickable
                                disabled={!testPhone || testing}
                                icon={testing ? <CircularProgress size={12} color='inherit' /> : <IconSend size={14} />}
                                onClick={runTest}
                            />
                        </Stack>
                        {testResult?.ok && (
                            <Alert severity='success'>
                                Terkirim. Pratinjau isi pesan:
                                <Box component='pre' sx={{ whiteSpace: 'pre-wrap', mt: 1, mb: 0 }}>
                                    {testResult.preview}
                                </Box>
                            </Alert>
                        )}
                        {testResult && !testResult.ok && <Alert severity='error'>{testResult.error}</Alert>}

                        <Divider />
                        <Paper variant='outlined' sx={{ p: 2 }}>
                            <Typography variant='subtitle2' gutterBottom>
                                Ringkasan
                            </Typography>
                            <Typography variant='body2'>Nama: {form.name || '—'}</Typography>
                            <Typography variant='body2'>Pengirim: {senders.find((s) => s.id === form.sender_id)?.label || '—'}</Typography>
                            <Typography variant='body2'>Gambar: {form.media_path ? 'ada' : 'tidak'}</Typography>
                            <Typography variant='body2'>Blacklist: {form.include_blacklist ? 'disertakan' : 'dikecualikan'}</Typography>
                            <Typography variant='body2'>Mode: {form.dry_run ? 'dry run' : 'kirim sungguhan'}</Typography>
                        </Paper>

                        <FormControlLabel
                            control={<Checkbox checked={confirmed} onChange={(e) => setConfirmed(e.target.checked)} />}
                            label='Saya sudah kirim tes dan meninjau isi pesan'
                        />
                    </Stack>
                )}

                <Divider sx={{ my: 2 }} />
                <Stack direction='row' spacing={1} justifyContent='flex-end'>
                    <Chip size='small' variant='outlined' label='Batal' clickable onClick={onClose} />
                    {step > 0 && <Chip size='small' variant='outlined' label='Kembali' clickable onClick={() => setStep(step - 1)} />}
                    {step < STEPS.length - 1 && (
                        <Chip size='small' color='primary' label='Lanjut' clickable onClick={() => setStep(step + 1)} />
                    )}
                    {step === STEPS.length - 1 && (
                        <Chip
                            size='small'
                            color='primary'
                            label={saving ? 'Menyimpan...' : 'Simpan Campaign'}
                            clickable
                            disabled={!confirmed || saving}
                            icon={saving ? <CircularProgress size={12} color='inherit' /> : undefined}
                            onClick={async () => {
                                const id = await save()
                                if (id) onClose()
                            }}
                        />
                    )}
                </Stack>
            </DialogContent>
        </Dialog>
    )
}

CampaignDialog.propTypes = {
    open: PropTypes.bool.isRequired,
    campaign: PropTypes.object,
    onClose: PropTypes.func.isRequired,
    onSaved: PropTypes.func.isRequired
}

export default CampaignDialog
