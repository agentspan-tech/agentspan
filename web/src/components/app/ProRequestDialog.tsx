import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { Loader2, Sparkles, Check } from 'lucide-react'
import { api } from '@/lib/api'
import { useI18n } from '@/i18n'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'

const inputClass = "w-full bg-zinc-900 border border-zinc-800 rounded-md px-3 py-2.5 text-sm text-zinc-100 placeholder:text-zinc-600 focus:outline-none focus:border-zinc-600 focus:ring-1 focus:ring-zinc-600 transition-colors"

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  source: string
}

export function ProRequestDialog({ open, onOpenChange, source }: Props) {
  const { t } = useI18n()
  const [email, setEmail] = useState('')
  const [company, setCompany] = useState('')
  const [message, setMessage] = useState('')
  const [submitted, setSubmitted] = useState(false)
  const [error, setError] = useState('')

  const mutation = useMutation({
    mutationFn: () => api.post('/auth/pro-request', { email, company, message, source }),
    onSuccess: () => setSubmitted(true),
    onError: () => setError(t.pro_dialog_error),
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    if (!email.trim()) return
    mutation.mutate()
  }

  function handleClose(open: boolean) {
    if (!open) {
      setTimeout(() => { setSubmitted(false); setError('') }, 200)
    }
    onOpenChange(open)
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="bg-zinc-950 border-zinc-800 max-w-md">
        <DialogHeader>
          <DialogTitle className="text-zinc-50">{t.pro_dialog_title}</DialogTitle>
        </DialogHeader>

        {submitted ? (
          <div className="text-center py-6">
            <div className="inline-flex items-center justify-center w-10 h-10 rounded-full bg-emerald-500/10 mb-3">
              <Check size={18} className="text-emerald-500" />
            </div>
            <p className="text-sm text-zinc-300">{t.pro_dialog_success}</p>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="space-y-4 pt-2">
            <p className="text-sm text-zinc-500">{t.pro_dialog_desc}</p>

            <div>
              <label className="block text-sm font-medium text-zinc-300 mb-1.5">{t.pro_dialog_email}</label>
              <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder={t.pro_dialog_email_placeholder} className={inputClass} required />
            </div>

            <div>
              <label className="block text-sm font-medium text-zinc-300 mb-1.5">{t.pro_dialog_company}</label>
              <input type="text" value={company} onChange={(e) => setCompany(e.target.value)} placeholder={t.pro_dialog_company_placeholder} className={inputClass} />
            </div>

            <div>
              <label className="block text-sm font-medium text-zinc-300 mb-1.5">{t.pro_dialog_message}</label>
              <textarea value={message} onChange={(e) => setMessage(e.target.value)} placeholder={t.pro_dialog_message_placeholder} rows={3} className={inputClass + ' resize-none'} />
            </div>

            {error && <p className="text-sm text-red-400">{error}</p>}

            <Button type="submit" disabled={mutation.isPending || !email.trim()} className="w-full bg-zinc-50 text-zinc-950 hover:bg-zinc-200">
              {mutation.isPending ? <><Loader2 size={14} className="animate-spin mr-2" />{t.pro_dialog_submitting}</> : t.pro_dialog_submit}
            </Button>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}

interface ProCTACardProps {
  title: string
  description: string
  source: string
}

export function ProCTACard({ title, description, source }: ProCTACardProps) {
  const { t } = useI18n()
  const [open, setOpen] = useState(false)

  return (
    <>
      <div className="text-center py-16 max-w-sm mx-auto">
        <div className="inline-flex items-center justify-center w-10 h-10 rounded-full bg-indigo-500/10 mb-4">
          <Sparkles size={18} className="text-indigo-400" />
        </div>
        <h3 className="text-base font-medium text-zinc-200 mb-2">{title}</h3>
        <p className="text-sm text-zinc-500 mb-6 leading-relaxed">{description}</p>
        <button
          onClick={() => setOpen(true)}
          className="inline-flex items-center gap-2 text-sm font-medium bg-indigo-500/10 text-indigo-400 border border-indigo-500/20 px-4 py-2 rounded-md hover:bg-indigo-500/20 transition-colors"
        >
          <Sparkles size={13} />
          {t.pro_cta_btn}
        </button>
      </div>
      <ProRequestDialog open={open} onOpenChange={setOpen} source={source} />
    </>
  )
}
