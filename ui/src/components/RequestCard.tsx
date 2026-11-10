import { useState, type FormEvent } from 'react'
import { answerRequest, type Answer } from '../api'
import type { PendingRequest, Question } from '../types'

interface RequestCardProps {
  conversationId: string
  request: PendingRequest
}

// RequestCard is answerable only because the service's pending set lists the
// request. the first answer from any browser wins; a later one is refused.
export function RequestCard({ conversationId, request }: RequestCardProps) {
  const [error, setError] = useState<string | null>(null)
  const [sending, setSending] = useState(false)

  const send = async (answer: Answer) => {
    setSending(true)
    setError(null)
    try {
      await answerRequest(conversationId, request.id, answer)
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure))
      setSending(false)
    }
  }

  return (
    <section className="request-card" aria-label={request.kind === 'question' ? 'Question' : 'Approval'}>
      {request.kind === 'question' ? (
        <QuestionForm questions={request.payload?.questions ?? []} sending={sending} onAnswer={(answers) => void send({ answers })} />
      ) : (
        <PermissionForm request={request} sending={sending} onAnswer={(answer) => void send(answer)} />
      )}
      {error && <p className="error">{error}</p>}
    </section>
  )
}

function describeInput(input: Record<string, unknown> | undefined): string {
  if (!input) {
    return ''
  }
  for (const key of ['command', 'file_path', 'path', 'url', 'pattern']) {
    if (typeof input[key] === 'string') {
      return input[key]
    }
  }
  return JSON.stringify(input, null, 2)
}

function PermissionForm({ request, sending, onAnswer }: { request: PendingRequest; sending: boolean; onAnswer: (answer: Answer) => void }) {
  const [message, setMessage] = useState('')
  const payload = request.payload ?? {}
  const detail = payload.title ?? payload.description ?? payload.decision_reason
  return (
    <>
      <h2>Approval: {payload.display_name ?? payload.tool ?? 'tool'}</h2>
      {payload.permission_mode === 'auto' && (
        <p className="muted">Raised in auto mode: the classifier could not decide this call, so it needs your answer.</p>
      )}
      {detail && <p>{detail}</p>}
      <pre>{describeInput(payload.input)}</pre>
      {payload.blocked_path && (
        <p className="muted">
          Path: <code>{payload.blocked_path}</code>
        </p>
      )}
      <div className="inline-form">
        <input placeholder="Reason, sent with a denial" value={message} onChange={(event) => setMessage(event.target.value)} />
        <button type="button" disabled={sending} onClick={() => onAnswer({ decision: 'allow' })}>
          Allow
        </button>
        <button type="button" disabled={sending} onClick={() => onAnswer({ decision: 'deny', message })}>
          Deny
        </button>
      </div>
    </>
  )
}

function QuestionForm({ questions, sending, onAnswer }: { questions: Question[]; sending: boolean; onAnswer: (answers: Record<string, string>) => void }) {
  const [chosen, setChosen] = useState<Record<string, string[]>>({})
  const [other, setOther] = useState<Record<string, string>>({})

  const toggle = (question: Question, label: string) => {
    setChosen((current) => {
      const picked = current[question.question] ?? []
      if (!question.multiSelect) {
        return { ...current, [question.question]: [label] }
      }
      return {
        ...current,
        [question.question]: picked.includes(label) ? picked.filter((l) => l !== label) : [...picked, label],
      }
    })
  }

  const answers: Record<string, string> = {}
  for (const question of questions) {
    const typed = other[question.question]?.trim()
    const picked = chosen[question.question] ?? []
    const value = typed ? [...picked, typed] : picked
    if (value.length > 0) {
      answers[question.question] = value.join(', ')
    }
  }
  const complete = questions.length > 0 && questions.every((question) => answers[question.question])

  const onSubmit = (event: FormEvent) => {
    event.preventDefault()
    if (complete) {
      onAnswer(answers)
    }
  }

  return (
    <form onSubmit={onSubmit}>
      <h2>Question</h2>
      {questions.map((question) => (
        <fieldset key={question.question}>
          <legend>
            {question.header && <span className="tag">{question.header}</span>} {question.question}
          </legend>
          {(question.options ?? []).map((option) => (
            <label key={option.label} className="option">
              <input
                type={question.multiSelect ? 'checkbox' : 'radio'}
                name={question.question}
                checked={(chosen[question.question] ?? []).includes(option.label)}
                onChange={() => toggle(question, option.label)}
              />
              <span>
                {option.label}
                {option.description && <span className="muted"> {option.description}</span>}
              </span>
            </label>
          ))}
          <input
            placeholder="Or write your own answer"
            value={other[question.question] ?? ''}
            onChange={(event) => setOther((current) => ({ ...current, [question.question]: event.target.value }))}
          />
        </fieldset>
      ))}
      <div className="actions">
        <button type="submit" disabled={sending || !complete}>
          Answer
        </button>
      </div>
    </form>
  )
}
