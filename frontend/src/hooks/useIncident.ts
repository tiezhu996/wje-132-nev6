import { useState } from 'react'
import { message } from 'antd'
import { assignIncident, rectifyIncident, reviewIncident, getIncident } from '@/api/incident'
import type { SafetyIncident } from '@/types'

export function useIncident() {
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

  async function assign(id: number) {
    await assignIncident(id)
    message.success('已指派调查')
    await load(id)
  }

  async function rectify(id: number, measures: string, deadline?: string) {
    await rectifyIncident(id, { measures, deadline })
    message.success('整改已提交，等待复核')
    await load(id)
  }

  async function review(id: number, approved: boolean, comment?: string) {
    try {
      await reviewIncident(id, { approved, comment })
      message.success(approved ? '验收通过，事件已关闭' : '已驳回，退回整改')
    } finally {
      // 无论成败都回读最新状态：并发/重复验收失败时展示已落库的复核意见
      await load(id)
    }
  }

  return { loading, incident, load, assign, rectify, review }
}
