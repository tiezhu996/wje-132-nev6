import { useState } from 'react'
import { message } from 'antd'
import { assignIncident, rectifyIncident, reviewIncident, closeIncident, getIncident } from '@/api/incident'
import type { SafetyIncident } from '@/types'

export function useIncident(onChanged?: () => void) {
  const [loading, setLoading] = useState(false)
  const [incident, setIncident] = useState<SafetyIncident | null>(null)

  async function load(id: number) {
    setLoading(true)
    try {
      const res: any = await getIncident(id)
      setIncident(res.data)
    } finally {
      setLoading(false)
    }
  }

  function reset() {
    setIncident(null)
  }

  async function assign(id: number) {
    await assignIncident(id)
    message.success('已指派调查')
    await load(id)
    onChanged?.()
  }

  async function rectify(id: number, measures?: string, deadline?: string) {
    await rectifyIncident(id, { measures, deadline })
    message.success('整改已提交，等待安全管理员复核')
    await load(id)
    onChanged?.()
  }

  async function review(id: number, approved: boolean, comment?: string) {
    await reviewIncident(id, { approved, comment })
    message.success(approved ? '复核通过，隐患已关闭' : '已驳回，退回整改')
    await load(id)
    onChanged?.()
  }

  async function close(id: number) {
    await closeIncident(id)
    message.success('事件已关闭')
    await load(id)
    onChanged?.()
  }

  return { loading, incident, load, reset, assign, rectify, review, close }
}
