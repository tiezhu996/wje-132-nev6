import { useEffect, useState } from 'react'
import { Alert, Button, Card, DatePicker, Form, Input, Modal, Select, Space, Table, message } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { reportIncident } from '@/api/incident'
import { useIncidentStore } from '@/stores/incidentStore'
import { useIncident } from '@/hooks/useIncident'
import RiskLevelTag from '@/components/common/RiskLevelTag'
import StatusBadge from '@/components/common/StatusBadge'
import RoleGuard from '@/components/common/RoleGuard'
import { IncidentCategories, IncidentStatusOptions, ReviewResult, ReviewResultText, SeverityOptions } from '@/constants/incident'
import { formatDateTime } from '@/utils/dateFormat'
import type { SafetyIncident } from '@/types'

export default function IncidentManage() {
  const store = useIncidentStore()
  const incident = useIncident()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [filters, setFilters] = useState<Record<string, unknown>>({})
  const [open, setOpen] = useState(false)
  const [detailId, setDetailId] = useState<number>()
  const [rectifyOpen, setRectifyOpen] = useState(false)
  const [reviewMode, setReviewMode] = useState<'approve' | 'reject' | null>(null)
  const [form] = Form.useForm()
  const [rectifyForm] = Form.useForm()
  const [reviewForm] = Form.useForm()

  useEffect(() => {
    store.fetchList({ page, page_size: pageSize, ...filters })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, pageSize, filters])

  useEffect(() => {
    if (detailId) incident.load(detailId)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [detailId])

  function refreshList() {
    store.fetchList({ page, page_size: pageSize, ...filters })
  }

  async function onReport() {
    const values = await form.validateFields()
    await reportIncident({
      title: values.title,
      description: values.description,
      occurred_at: values.occurred_at ? values.occurred_at.format('YYYY-MM-DDTHH:mm:ss') : undefined,
      site_id: values.site_id,
      area: values.area,
      severity_level: values.severity_level,
      category: values.category,
    })
    message.success('事件上报成功')
    setOpen(false)
    form.resetFields()
    setPage(1)
  }

  async function onRectify() {
    const values = await rectifyForm.validateFields()
    if (!detailId) return
    try {
      await incident.rectify(
        detailId,
        values.measures,
        values.deadline ? values.deadline.format('YYYY-MM-DDTHH:mm:ss') : undefined,
      )
    } catch {
      // 拦截器已提示（如状态冲突），详情已回读最新状态
    }
    setRectifyOpen(false)
    rectifyForm.resetFields()
    refreshList()
  }

  async function onReview() {
    const values = await reviewForm.validateFields()
    if (!detailId || !reviewMode) return
    try {
      await incident.review(detailId, reviewMode === 'approve', values.comment)
    } catch {
      // 拦截器已提示（如重复验收冲突），详情已回读最新复核意见
    }
    setReviewMode(null)
    reviewForm.resetFields()
    refreshList()
  }

  return (
    <Card>
      <Space style={{ marginBottom: 16 }}>
        <Select placeholder="严重等级" allowClear style={{ width: 130 }} options={SeverityOptions} onChange={(v) => { setFilters({ severity: v }); setPage(1) }} />
        <Select placeholder="状态" allowClear style={{ width: 130 }} options={IncidentStatusOptions} onChange={(v) => { setFilters({ status: v }); setPage(1) }} />
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setOpen(true)}>上报事件</Button>
      </Space>
      <Table<SafetyIncident>
        rowKey="id"
        dataSource={store.list}
        pagination={{ current: page, pageSize, total: store.total, onChange: (p, ps) => { setPage(p); setPageSize(ps) } }}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 70 },
          { title: '标题', dataIndex: 'title' },
          { title: '区域', dataIndex: 'area' },
          { title: '风险等级', dataIndex: 'severity_level', render: (v) => <RiskLevelTag level={v} /> },
          { title: '分类', dataIndex: 'category' },
          { title: '状态', dataIndex: 'status', render: (v) => <StatusBadge status={v} /> },
          { title: '复核人', dataIndex: 'reviewer_name', width: 90, render: (v) => v || '-' },
          {
            title: '复核意见/驳回原因',
            dataIndex: 'review_comment',
            ellipsis: true,
            render: (v, row) => (row.review_result ? v || '-' : '-'),
          },
          { title: '发生时间', dataIndex: 'occurred_at', render: (v) => formatDateTime(v) },
          {
            title: '操作',
            render: (_, row) => <a onClick={() => setDetailId(row.id)}>详情</a>,
          },
        ]}
      />
      <Modal title="上报安全事件" open={open} onOk={onReport} onCancel={() => setOpen(false)} width={560}>
        <Form form={form} layout="vertical">
          <Form.Item name="title" label="事件标题" rules={[{ required: true }]}><Input /></Form.Item>
          <Form.Item name="description" label="事件描述"><Input.TextArea rows={3} /></Form.Item>
          <Form.Item name="occurred_at" label="发生时间" rules={[{ required: true }]}><DatePicker showTime style={{ width: '100%' }} /></Form.Item>
          <Form.Item name="site_id" label="工地编号"><Input /></Form.Item>
          <Form.Item name="area" label="区域"><Input /></Form.Item>
          <Form.Item name="severity_level" label="严重等级" rules={[{ required: true }]}><Select options={SeverityOptions} /></Form.Item>
          <Form.Item name="category" label="分类" rules={[{ required: true }]}><Select options={IncidentCategories.map((c) => ({ label: c, value: c }))} /></Form.Item>
        </Form>
      </Modal>
      <Modal title="事件详情" open={!!detailId} onCancel={() => setDetailId(undefined)} footer={null} width={640}>
        {incident.incident && (() => {
          const cur = incident.incident
          return (
            <div>
              <p><b>{cur.title}</b> <RiskLevelTag level={cur.severity_level} /> <StatusBadge status={cur.status} /></p>
              <p>{cur.description}</p>
              <p>区域：{cur.area} / 分类：{cur.category}</p>
              <p>整改措施：{cur.rectification_measures || '-'}</p>
              <p>整改截止时间：{formatDateTime(cur.rectification_deadline)}</p>
              {cur.review_result && (
                <Alert
                  style={{ marginBottom: 16 }}
                  type={cur.review_result === ReviewResult.REJECTED ? 'error' : 'success'}
                  message={`复核结果：${ReviewResultText[cur.review_result] || cur.review_result}（复核人：${cur.reviewer_name || '-'}，时间：${formatDateTime(cur.reviewed_at)}）`}
                  description={`${cur.review_result === ReviewResult.REJECTED ? '驳回原因' : '复核意见'}：${cur.review_comment || '-'}`}
                />
              )}
              <RoleGuard roles={['admin', 'safety_manager']}>
                <Space>
                  {cur.status === 'reported' && <Button type="primary" onClick={async () => { await incident.assign(cur.id); refreshList() }}>指派调查</Button>}
                  {cur.status === 'investigating' && <Button type="primary" onClick={() => setRectifyOpen(true)}>提交整改</Button>}
                </Space>
              </RoleGuard>
              <RoleGuard roles={['safety_manager']}>
                {cur.status === 'pending_review' && (
                  <Space>
                    <Button type="primary" onClick={() => setReviewMode('approve')}>验收通过</Button>
                    <Button danger onClick={() => setReviewMode('reject')}>驳回</Button>
                  </Space>
                )}
              </RoleGuard>
            </div>
          )
        })()}
      </Modal>
      <Modal
        title="提交整改"
        open={rectifyOpen}
        onOk={onRectify}
        onCancel={() => setRectifyOpen(false)}
        okText="提交并送复核"
        width={520}
      >
        <Alert style={{ marginBottom: 16 }} type="info" message="提交后事件进入待复核，由安全管理员验收通过后关闭。" />
        <Form form={rectifyForm} layout="vertical">
          <Form.Item name="measures" label="整改措施" rules={[{ required: true, message: '请填写整改措施' }]}>
            <Input.TextArea rows={3} />
          </Form.Item>
          <Form.Item name="deadline" label="整改截止时间">
            <DatePicker showTime style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>
      <Modal
        title={reviewMode === 'approve' ? '验收通过' : '驳回整改'}
        open={!!reviewMode}
        onOk={onReview}
        onCancel={() => setReviewMode(null)}
        okText={reviewMode === 'approve' ? '确认验收通过' : '确认驳回'}
        okButtonProps={reviewMode === 'reject' ? { danger: true } : undefined}
        width={520}
      >
        <Form form={reviewForm} layout="vertical">
          <Form.Item
            name="comment"
            label={reviewMode === 'reject' ? '驳回原因' : '复核意见（选填）'}
            rules={reviewMode === 'reject' ? [{ required: true, whitespace: true, message: '驳回必须填写原因' }] : undefined}
          >
            <Input.TextArea rows={3} placeholder={reviewMode === 'reject' ? '请说明驳回原因，事件将退回整改中' : '可填写验收意见'} />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  )
}
