import { useEffect, useState } from 'react'
import { Alert, Button, Card, DatePicker, Descriptions, Form, Input, Modal, Popconfirm, Select, Space, Table, Tag, message } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import { reportIncident } from '@/api/incident'
import { useIncidentStore } from '@/stores/incidentStore'
import { useIncident } from '@/hooks/useIncident'
import RiskLevelTag from '@/components/common/RiskLevelTag'
import StatusBadge from '@/components/common/StatusBadge'
import RoleGuard from '@/components/common/RoleGuard'
import { IncidentCategories, IncidentStatus, IncidentStatusOptions, ReviewResult, SeverityOptions } from '@/constants/incident'
import { formatDateTime } from '@/utils/dateFormat'
import type { SafetyIncident } from '@/types'

// 列表中展示待复核/驳回原因等复核信息。
function ReviewCell({ row }: { row: SafetyIncident }) {
  if (row.status === IncidentStatus.REVIEW_PENDING) {
    return <Tag color="gold">待复核</Tag>
  }
  if (row.review_result === ReviewResult.REJECTED) {
    return (
      <Space size={4}>
        <Tag color="red">已驳回</Tag>
        <span title={row.review_comment} style={{ maxWidth: 140, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', display: 'inline-block' }}>
          {row.review_comment}
        </span>
      </Space>
    )
  }
  if (row.review_result === ReviewResult.APPROVED) {
    return <Tag color="green">复核通过</Tag>
  }
  return <span>-</span>
}

export default function IncidentManage() {
  const store = useIncidentStore()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [filters, setFilters] = useState<Record<string, unknown>>({})
  const [open, setOpen] = useState(false)
  const [detailId, setDetailId] = useState<number>()
  const [form] = Form.useForm()
  const [rectifyOpen, setRectifyOpen] = useState(false)
  const [rectifyTarget, setRectifyTarget] = useState<SafetyIncident | null>(null)
  const [rectifyForm] = Form.useForm()
  const [rejectOpen, setRejectOpen] = useState(false)
  const [rejectForm] = Form.useForm()
  const incident = useIncident(() => {
    store.fetchList({ page, page_size: pageSize, ...filters })
  })

  useEffect(() => {
    store.fetchList({ page, page_size: pageSize, ...filters })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, pageSize, filters])

  useEffect(() => {
    if (detailId) {
      incident.load(detailId)
    } else {
      incident.reset()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [detailId])

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

  function openRectify(target: SafetyIncident) {
    setRectifyTarget(target)
    rectifyForm.setFieldsValue({
      measures: target.rectification_measures || '',
      deadline: target.rectification_deadline ? dayjs(target.rectification_deadline) : undefined,
    })
    setRectifyOpen(true)
  }

  async function onRectifyOk() {
    const v = await rectifyForm.validateFields()
    if (!rectifyTarget) return
    await incident.rectify(
      rectifyTarget.id,
      v.measures,
      v.deadline ? v.deadline.format('YYYY-MM-DDTHH:mm:ss') : undefined,
    )
    setRectifyOpen(false)
    setRectifyTarget(null)
    rectifyForm.resetFields()
  }

  function openReject() {
    rejectForm.resetFields()
    setRejectOpen(true)
  }

  async function onRejectOk() {
    const v = await rejectForm.validateFields()
    if (!detailId) return
    await incident.review(detailId, false, v.comment)
    setRejectOpen(false)
    rejectForm.resetFields()
  }

  const cur = incident.incident
  const rectifyIsReSubmit = rectifyTarget?.review_result === ReviewResult.REJECTED

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
          { title: '复核', width: 220, render: (_, row) => <ReviewCell row={row} /> },
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
      <Modal
        title="事件详情"
        open={!!detailId}
        onCancel={() => setDetailId(undefined)}
        footer={null}
        width={680}
      >
        {cur && (
          <div>
            <p><b>{cur.title}</b> <RiskLevelTag level={cur.severity_level} /> <StatusBadge status={cur.status} /></p>
            <p>{cur.description}</p>
            <Descriptions column={1} size="small" bordered style={{ marginTop: 12 }}>
              <Descriptions.Item label="区域">{cur.area} / {cur.category}</Descriptions.Item>
              <Descriptions.Item label="整改措施">{cur.rectification_measures || '-'}</Descriptions.Item>
              <Descriptions.Item label="整改截止时间">{cur.rectification_deadline ? formatDateTime(cur.rectification_deadline) : '-'}</Descriptions.Item>
              <Descriptions.Item label="复核结论">
                {cur.review_result === ReviewResult.APPROVED && <Tag color="green">验收通过</Tag>}
                {cur.review_result === ReviewResult.REJECTED && <Tag color="red">驳回整改</Tag>}
                {cur.status === IncidentStatus.REVIEW_PENDING && <Tag color="gold">待复核</Tag>}
                {(!cur.review_result && cur.status !== IncidentStatus.REVIEW_PENDING) && '-'}
              </Descriptions.Item>
              <Descriptions.Item label="驳回原因">
                {cur.review_result === ReviewResult.REJECTED ? (cur.review_comment || '-') : '-'}
              </Descriptions.Item>
              <Descriptions.Item label="复核人">
                {cur.reviewer_name
                  ? `${cur.reviewer_name}${cur.reviewed_at ? `（${formatDateTime(cur.reviewed_at)}）` : ''}`
                  : '-'}
              </Descriptions.Item>
            </Descriptions>

            {cur.review_result === ReviewResult.REJECTED && cur.status === IncidentStatus.INVESTIGATING && (
              <Alert
                style={{ marginTop: 12 }}
                type="warning"
                showIcon
                message={`复核驳回：${cur.review_comment}`}
                description="请按驳回原因继续整改后重新提交；原整改措施与截止时间已保留。"
              />
            )}

            <RoleGuard roles={['admin', 'safety_manager']}>
              <Space style={{ marginTop: 16 }}>
                {cur.status === IncidentStatus.REPORTED && (
                  <Button type="primary" onClick={() => incident.assign(cur.id)}>指派调查</Button>
                )}
                {cur.status === IncidentStatus.INVESTIGATING && (
                  <Button type="primary" onClick={() => openRectify(cur)}>提交整改</Button>
                )}
                {cur.status === IncidentStatus.REVIEW_PENDING && (
                  <RoleGuard roles={['safety_manager']}>
                    <Space>
                      <Popconfirm
                        title="确认验收通过？"
                        description="验收通过后隐患将关闭。"
                        onConfirm={() => incident.review(cur.id, true)}
                        okText="通过并关闭"
                        cancelText="取消"
                      >
                        <Button type="primary">验收通过</Button>
                      </Popconfirm>
                      <Button danger onClick={openReject}>驳回整改</Button>
                    </Space>
                  </RoleGuard>
                )}
                {cur.status === IncidentStatus.RESOLVED && (
                  <Button danger onClick={() => incident.close(cur.id)}>关闭事件</Button>
                )}
              </Space>
            </RoleGuard>
          </div>
        )}
      </Modal>

      <Modal
        title={rectifyIsReSubmit ? '重新提交整改' : '提交整改'}
        open={rectifyOpen}
        onOk={onRectifyOk}
        onCancel={() => { setRectifyOpen(false); setRectifyTarget(null); rectifyForm.resetFields() }}
        okText="提交整改"
        width={560}
      >
        {rectifyIsReSubmit && (
          <Alert
            type="warning"
            showIcon
            style={{ marginBottom: 12 }}
            message={`上次复核驳回原因：${rectifyTarget?.review_comment || '无'}`}
          />
        )}
        {!rectifyIsReSubmit && (
          <Alert type="info" showIcon style={{ marginBottom: 12 }} message="提交后进入待复核，需安全管理员验收通过方可关闭。" />
        )}
        <Form form={rectifyForm} layout="vertical">
          <Form.Item name="measures" label="整改措施" rules={[{ required: true, whitespace: true, message: '请填写整改措施' }]}>
            <Input.TextArea rows={3} placeholder="驳回后默认保留原措施，也可补充新的整改措施" />
          </Form.Item>
          <Form.Item name="deadline" label="整改截止时间">
            <DatePicker showTime style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="驳回整改"
        open={rejectOpen}
        onOk={onRejectOk}
        onCancel={() => setRejectOpen(false)}
        okText="确认驳回"
        okButtonProps={{ danger: true }}
        width={480}
      >
        <Alert type="warning" showIcon style={{ marginBottom: 12 }} message="驳回后退回整改中，需重新提交整改；原措施与截止时间保留。" />
        <Form form={rejectForm} layout="vertical">
          <Form.Item
            name="comment"
            label="驳回原因"
            rules={[
              { required: true, whitespace: true, message: '驳回必须填写原因' },
              { max: 500, message: '驳回原因不超过 500 字' },
            ]}
          >
            <Input.TextArea rows={4} placeholder="请说明不通过的具体原因，便于整改人对照修改" />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  )
}
