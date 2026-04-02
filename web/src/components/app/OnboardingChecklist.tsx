import { useState } from 'react'
import { Link } from 'react-router-dom'
import { Check, Copy } from 'lucide-react'
import { useI18n } from '@/i18n'
import { useAPIKeys } from '@/hooks/use-keys'
import { cn } from '@/lib/utils'

interface OnboardingChecklistProps {
  hasSessions: boolean
  orgID: string
}

export function OnboardingChecklist({ hasSessions, orgID }: OnboardingChecklistProps) {
  const { t } = useI18n()
  const { data: keys } = useAPIKeys(orgID)
  const hasKey = (keys?.length ?? 0) > 0
  const step1Done = hasKey
  const step2Done = hasKey
  const step3Done = hasKey
  const step4Done = hasSessions

  if (step4Done) return null

  const baseUrl = `${window.location.origin}/v1`

  return (
    <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-6">
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-xs font-medium text-zinc-500 uppercase tracking-wider">{t.onboarding_title}</h2>
        <span className="text-xs text-zinc-600 tabular-nums">{[step1Done, step2Done, step3Done, step4Done].filter(Boolean).length}/4</span>
      </div>
      <div className="space-y-5">
        <ChecklistStep num={1} done={step1Done} label={t.onboarding_step1}>
          {!step1Done && (
            <Link to="/keys" className="mt-1.5 inline-flex text-sm text-indigo-400/80 hover:text-indigo-400 transition-colors">
              {t.onboarding_step1_link}
            </Link>
          )}
        </ChecklistStep>
        <ChecklistStep num={2} done={step2Done} label={t.onboarding_step2}>
          <div className="mt-2"><CopyableCodeBlock value={baseUrl} /></div>
        </ChecklistStep>
        <ChecklistStep num={3} done={step3Done} label={t.onboarding_step3}>
          <div className="mt-2 space-y-2">
            <div>
              <p className="text-[10px] font-medium uppercase tracking-wider text-zinc-600 mb-1">{t.onboarding_step3_before}</p>
              <CodeBlock value={`base_url="https://api.openai.com/v1"`} />
            </div>
            <div>
              <p className="text-[10px] font-medium uppercase tracking-wider text-zinc-600 mb-1">{t.onboarding_step3_after}</p>
              <CopyableCodeBlock value={`base_url="${baseUrl}"`} />
            </div>
          </div>
        </ChecklistStep>
        <ChecklistStep num={4} done={step4Done} label={t.onboarding_step4} />
      </div>
    </div>
  )
}

interface ChecklistStepProps { num: number; done: boolean; label: string; children?: React.ReactNode }

function ChecklistStep({ num, done, label, children }: ChecklistStepProps) {
  return (
    <div className="flex gap-3">
      <div className="shrink-0 mt-0.5">
        {done ? (
          <div className="w-5 h-5 rounded-full bg-emerald-500/10 flex items-center justify-center">
            <Check size={12} className="text-emerald-500" />
          </div>
        ) : (
          <div className="w-5 h-5 rounded-full bg-zinc-800 flex items-center justify-center">
            <span className="text-[10px] font-mono text-zinc-500 tabular-nums">{num}</span>
          </div>
        )}
      </div>
      <div className="flex-1 min-w-0">
        <p className={cn('text-sm', done ? 'text-zinc-600 line-through' : 'text-zinc-200')}>{label}</p>
        {children}
      </div>
    </div>
  )
}

function CodeBlock({ value }: { value: string }) {
  return (
    <div className="bg-zinc-950 border border-zinc-800 rounded-md p-3 font-mono text-sm text-zinc-400 overflow-x-auto">
      {value}
    </div>
  )
}

function CopyableCodeBlock({ value }: { value: string }) {
  const [copied, setCopied] = useState(false)
  function handleCopy() {
    navigator.clipboard.writeText(value).catch(() => {})
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }
  return (
    <div className="relative bg-zinc-950 border border-zinc-800 rounded-md p-3 pr-10 font-mono text-sm text-zinc-400 overflow-x-auto">
      {value}
      <button onClick={handleCopy} title="Copy" className="absolute right-2 top-1/2 -translate-y-1/2 text-zinc-600 hover:text-zinc-300 transition-colors">
        {copied ? <Check size={14} className="text-emerald-500" /> : <Copy size={14} />}
      </button>
    </div>
  )
}
