import { useState } from 'react'
import { Check, Copy } from 'lucide-react'

type Tab = 'openai' | 'anthropic' | 'curl'

interface Props {
  labels: { openai: string; anthropic: string; curl: string }
  copyLabel: string
  copiedLabel: string
}

const CODE: Record<Tab, { before: string; after: string }> = {
  openai: {
    before: `from openai import OpenAI

client = OpenAI(
  api_key="sk-...",
)`,
    after: `from openai import OpenAI

client = OpenAI(
  base_url="https://api.agentorbit.tech/v1",
  api_key="al-your-key",
)`,
  },
  anthropic: {
    before: `import anthropic

client = anthropic.Anthropic(
  api_key="sk-ant-...",
)`,
    after: `import anthropic

client = anthropic.Anthropic(
  base_url="https://api.agentorbit.tech/v1",
  api_key="al-your-key",
)`,
  },
  curl: {
    before: `curl https://api.openai.com/v1/chat/completions \\
  -H "Authorization: Bearer sk-..." \\
  -d '{"model":"gpt-4o","messages":[...]}'`,
    after: `curl https://api.agentorbit.tech/v1/chat/completions \\
  -H "Authorization: Bearer al-your-key" \\
  -d '{"model":"gpt-4o","messages":[...]}'`,
  },
}

function highlight(code: string, changed: 'before' | 'after') {
  const lines = code.split('\n')
  return lines.map((line, i) => {
    const isChanged =
      (changed === 'before' && (line.includes('api.openai.com') || line.includes('api_key="sk-') || line.includes('api_key="sk-ant-') || line.includes('Authorization: Bearer sk-'))) ||
      (changed === 'after'  && (line.includes('api.agentorbit.tech') || line.includes('api_key="al-') || line.includes('Authorization: Bearer al-')))

    return (
      <div
        key={i}
        className={`px-5 py-px transition-colors duration-200 ${
          isChanged
            ? changed === 'before'
              ? 'bg-red-500/[0.08] text-red-400'
              : 'bg-emerald-500/[0.08] text-emerald-400'
            : 'text-zinc-400'
        }`}
      >
        <span className="select-none text-zinc-800 mr-5 text-[10px] font-mono tabular-nums">{String(i + 1).padStart(2, ' ')}</span>
        {line || ' '}
      </div>
    )
  })
}

export function IntegrationCodeBlock({ labels, copyLabel, copiedLabel }: Props) {
  const [tab, setTab] = useState<Tab>('openai')
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    try { await navigator.clipboard.writeText(CODE[tab].after) } catch { /* clipboard unavailable */ }
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-950 overflow-hidden">
      {/* Tab bar */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-2 border-b border-zinc-800 px-3 sm:px-4 py-2.5 bg-zinc-900/40">
        <div className="flex gap-0.5 overflow-x-auto w-full sm:w-auto">
          {(['openai', 'anthropic', 'curl'] as Tab[]).map((t) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={`px-3 py-1 rounded text-xs whitespace-nowrap transition-colors duration-150 ${
                tab === t ? 'bg-zinc-800 text-zinc-100' : 'text-zinc-500 hover:text-zinc-300'
              }`}
            >{labels[t]}</button>
          ))}
        </div>
        <button
          onClick={handleCopy}
          className="flex items-center gap-1.5 text-xs text-zinc-500 hover:text-zinc-300 transition-colors duration-150 shrink-0"
        >
          {copied ? <Check size={11} className="text-emerald-500" /> : <Copy size={11} />}
          {copied ? copiedLabel : copyLabel}
        </button>
      </div>

      {/* Code comparison */}
      <div className="grid md:grid-cols-2 divide-y md:divide-y-0 md:divide-x divide-zinc-800">
        <div>
          <div className="px-5 py-2 border-b border-zinc-800 flex items-center gap-2">
            <span className="w-1.5 h-1.5 rounded-full bg-zinc-700" />
            <span className="text-xs text-zinc-500 font-mono">before</span>
          </div>
          <pre className="py-4 text-xs sm:text-sm font-mono leading-5 sm:leading-6 overflow-x-auto">
            {highlight(CODE[tab].before, 'before')}
          </pre>
        </div>
        <div>
          <div className="px-5 py-2 border-b border-zinc-800 flex items-center gap-2">
            <span className="w-1.5 h-1.5 rounded-full bg-emerald-600/70" />
            <span className="text-xs text-zinc-500 font-mono">after · AgentOrbit</span>
          </div>
          <pre className="py-4 text-xs sm:text-sm font-mono leading-5 sm:leading-6 overflow-x-auto">
            {highlight(CODE[tab].after, 'after')}
          </pre>
        </div>
      </div>
    </div>
  )
}
