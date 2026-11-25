import type { ReactNode } from 'react'
import clsx from 'clsx'
import { Info } from 'lucide-react'

type BadgeTone = 'success' | 'warning' | 'blocking' | 'neutral'

export function Eyebrow({ children }: { children: ReactNode }) {
  return <span className="text-primary text-xs font-bold tracking-[0.13em]">{children}</span>
}

export function PrimaryButton({ children, onClick, disabled, className }: { children: ReactNode; onClick?: () => void; disabled?: boolean; className?: string }) {
  return (
    <button
      className={clsx('inline-flex items-center justify-center gap-2 rounded-[9px] border-0 bg-primary px-4 py-[11px] text-sm font-semibold text-white transition-[background,transform] duration-150 hover:enabled:-translate-y-px hover:enabled:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-45', className)}
      disabled={disabled}
      onClick={onClick}
      type="button"
    >
      {children}
    </button>
  )
}

export function StatusBadge({ children, tone }: { children: ReactNode; tone: BadgeTone }) {
  return (
    <span className={clsx(
      'inline-flex w-fit items-center gap-1.5 rounded-full px-2.5 py-1.5 text-xs font-bold',
      tone === 'success' && 'bg-success-soft text-success',
      tone === 'warning' && 'bg-warning-soft text-warning',
      tone === 'blocking' && 'bg-danger-soft text-danger',
      tone === 'neutral' && 'bg-surface-muted text-text-secondary',
    )}>
      {children}
    </span>
  )
}

export function InformationNotice({ children }: { children: ReactNode }) {
  return (
    <div className="flex items-start gap-3 rounded-xl border border-primary-border bg-primary-soft p-4 text-sm leading-relaxed text-text-secondary">
      <Info className="mt-0.5 flex-none text-primary" size={19} />
      <span>{children}</span>
    </div>
  )
}
