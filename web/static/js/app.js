// OpenStack Reporter Web App

class OpenStackReporter {
	constructor() {
		this.data = null;
		this.filteredData = [];
		this.currentPage = 1;
		this.itemsPerPage = 20;
		this.init();
	}

	init() {
		this.bindEvents();
		this.loadData();
	}

	bindEvents() {
		document.getElementById('refreshBtn').addEventListener('click', () => this.refreshData());
		document.getElementById('exportPdfBtn').addEventListener('click', () => this.exportToPDF());
		document.getElementById('groupBy').addEventListener('change', () => this.applyFiltersAndSort());
		document.getElementById('sortBy').addEventListener('change', () => this.applyFiltersAndSort());
		document.getElementById('filterType').addEventListener('change', () => this.applyFiltersAndSort());
	}

	async loadData() {
		try {
			this.showLoading(true);
			const response = await fetch('/api/resources');

			if (!response.ok) {
				throw new Error(`HTTP error! status: ${response.status}`);
			}

			this.data = await response.json();
			this.updateSummary();
			this.applyFiltersAndSort();
			this.showLastUpdate();
			this.hideError();
		} catch (error) {
			console.error('Error loading data:', error);
			this.showError('Ошибка загрузки данных: ' + error.message);
		} finally {
			this.showLoading(false);
		}
	}

	async refreshData() {
		try {
			this.showLoading(true);
			const response = await fetch('/api/refresh', { method: 'POST' });

			if (!response.ok) {
				throw new Error(`HTTP error! status: ${response.status}`);
			}

			await this.loadData();
		} catch (error) {
			console.error('Error refreshing data:', error);
			this.showError('Ошибка обновления данных: ' + error.message);
			this.showLoading(false);
		}
	}

	async exportToPDF() {
		try {
			const response = await fetch('/api/export/pdf');

			if (!response.ok) {
				throw new Error(`HTTP error! status: ${response.status}`);
			}

			const blob = await response.blob();
			const url = window.URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = `openstack_report_${new Date().toISOString().split('T')[0]}.pdf`;
			document.body.appendChild(a);
			a.click();
			document.body.removeChild(a);
			window.URL.revokeObjectURL(url);
		} catch (error) {
			console.error('Error exporting PDF:', error);
			this.showError('Ошибка экспорта PDF: ' + error.message);
		}
	}

	applyFiltersAndSort() {
		if (!this.data || !this.data.resources) return;

		let filtered = [...this.data.resources];

		// Apply type filter
		const filterType = document.getElementById('filterType').value;
		if (filterType) {
			filtered = filtered.filter(resource => resource.type === filterType);
		}

		// Apply sorting
		const sortBy = document.getElementById('sortBy').value;
		const isDesc = sortBy.endsWith('_desc');
		const sortField = isDesc ? sortBy.replace('_desc', '') : sortBy;

		filtered.sort((a, b) => {
			let aValue = a[sortField];
			let bValue = b[sortField];

			if (sortField === 'created_at') {
				aValue = new Date(aValue);
				bValue = new Date(bValue);
			}

			let result;
			if (aValue < bValue) result = -1;
			else if (aValue > bValue) result = 1;
			else result = 0;

			// Reverse if desc
			return isDesc ? -result : result;
		});

		this.filteredData = filtered;
		this.currentPage = 1;
		this.renderTable();
		this.renderPagination();
	}

	renderTable() {
		const tbody = document.getElementById('resourcesTableBody');
		const groupBy = document.getElementById('groupBy').value;

		if (groupBy === 'project' || groupBy === 'type' || groupBy === 'status') {
			this.renderGroupedTable(tbody, groupBy);
		} else {
			this.renderFlatTable(tbody);
		}
	}

	renderGroupedTable(tbody, groupBy) {
		tbody.innerHTML = '';

		// Group resources
		const groups = {};
		this.filteredData.forEach(resource => {
			const key = groupBy === 'project' ? resource.project_name : resource[groupBy];
			if (!groups[key]) {
				groups[key] = [];
			}
			groups[key].push(resource);
		});

		// Render groups
		Object.keys(groups).sort().forEach(groupName => {
			// Group header
			const headerRow = document.createElement('tr');
			headerRow.className = 'table-secondary';
			headerRow.innerHTML = `
                <td colspan="6">
                    <strong>
                        <i class="fas fa-${this.getGroupIcon(groupBy)} me-2"></i>
                        ${groupName} (${groups[groupName].length})
                    </strong>
                </td>
            `;
			tbody.appendChild(headerRow);

			// Group items
			groups[groupName].forEach(resource => {
				tbody.appendChild(this.createResourceRow(resource));
			});
		});
	}

	renderFlatTable(tbody) {
		tbody.innerHTML = '';

		const startIndex = (this.currentPage - 1) * this.itemsPerPage;
		const endIndex = startIndex + this.itemsPerPage;
		const pageData = this.filteredData.slice(startIndex, endIndex);

		pageData.forEach(resource => {
			tbody.appendChild(this.createResourceRow(resource));
		});
	}

	createResourceRow(resource) {
		const row = document.createElement('tr');

		const createdDate = new Date(resource.created_at).toLocaleDateString('ru-RU');
		const statusClass = this.getStatusClass(resource.status);
		const typeClass = this.getTypeClass(resource.type);

		row.innerHTML = `
            <td>
                <strong>${resource.name || 'Без имени'}</strong>
                <br>
                <small class="text-muted">${this.getResourceSubtitle(resource)}</small>
            </td>
            <td>
                <span class="resource-type-badge ${typeClass}">
                    ${this.getTypeDisplayName(resource.type)}
                </span>
            </td>
            <td>${resource.project_name || 'Неизвестно'}</td>
            <td>
                <span class="status-badge ${statusClass}">
                    ${resource.status}
                </span>
            </td>
            <td>${createdDate}</td>
            <td>
                <button class="btn btn-sm btn-outline-primary" onclick="app.showResourceDetails('${resource.id}')">
                    <i class="fas fa-eye"></i>
                </button>
            </td>
        `;

		return row;
	}

	renderPagination() {
		const totalPages = Math.ceil(this.filteredData.length / this.itemsPerPage);
		const pagination = document.getElementById('pagination');
		const paginationNav = document.getElementById('paginationNav');

		if (totalPages <= 1) {
			paginationNav.style.display = 'none';
			return;
		}

		paginationNav.style.display = 'block';
		pagination.innerHTML = '';

		// Previous button
		const prevItem = document.createElement('li');
		prevItem.className = `page-item ${this.currentPage === 1 ? 'disabled' : ''}`;
		prevItem.innerHTML = `
            <a class="page-link" href="#" onclick="app.changePage(${this.currentPage - 1})">
                <i class="fas fa-chevron-left"></i>
            </a>
        `;
		pagination.appendChild(prevItem);

		// Page numbers
		const startPage = Math.max(1, this.currentPage - 2);
		const endPage = Math.min(totalPages, this.currentPage + 2);

		for (let i = startPage; i <= endPage; i++) {
			const pageItem = document.createElement('li');
			pageItem.className = `page-item ${i === this.currentPage ? 'active' : ''}`;
			pageItem.innerHTML = `
                <a class="page-link" href="#" onclick="app.changePage(${i})">${i}</a>
            `;
			pagination.appendChild(pageItem);
		}

		// Next button
		const nextItem = document.createElement('li');
		nextItem.className = `page-item ${this.currentPage === totalPages ? 'disabled' : ''}`;
		nextItem.innerHTML = `
            <a class="page-link" href="#" onclick="app.changePage(${this.currentPage + 1})">
                <i class="fas fa-chevron-right"></i>
            </a>
        `;
		pagination.appendChild(nextItem);
	}

	changePage(page) {
		const totalPages = Math.ceil(this.filteredData.length / this.itemsPerPage);

		if (page < 1 || page > totalPages) return;

		this.currentPage = page;
		this.renderTable();
		this.renderPagination();
	}

	showResourceDetails(resourceId) {
		const resource = this.data.resources.find(r => r.id === resourceId);
		if (!resource) return;

		const modal = new bootstrap.Modal(document.getElementById('resourceModal'));
		const modalTitle = document.getElementById('modalTitle');
		const modalBody = document.getElementById('modalBody');

		modalTitle.textContent = `${resource.name || 'Ресурс'} (${this.getTypeDisplayName(resource.type)})`;

		modalBody.innerHTML = `
            <div class="resource-details">
                <h6>Основная информация</h6>
                <p><strong>ID:</strong> ${resource.id}</p>
                <p><strong>Имя:</strong> ${resource.name || 'Не указано'}</p>
                <p><strong>Тип:</strong> ${this.getTypeDisplayName(resource.type)}</p>
                <p><strong>Проект:</strong> ${resource.project_name}</p>
                <p><strong>Статус:</strong> ${resource.status}</p>
                <p><strong>Создан:</strong> ${new Date(resource.created_at).toLocaleString('ru-RU')}</p>
                ${resource.updated_at ? `<p><strong>Обновлен:</strong> ${new Date(resource.updated_at).toLocaleString('ru-RU')}</p>` : ''}
            </div>
            ${this.renderResourceProperties(resource)}
        `;

		modal.show();
	}

	renderResourceProperties(resource) {
		if (!resource.properties) return '';

		const props = resource.properties;
		let html = '<div class="resource-details"><h6>Дополнительные свойства</h6>';

		switch (resource.type) {
			case 'server':
				html += `
                    <p><strong>Flavor:</strong> ${props.flavor_name || 'Unknown'}</p>
                    ${props.flavor_id ? `<p><strong>Flavor ID:</strong> ${props.flavor_id}</p>` : ''}

                    <p><strong>Сети:</strong></p>
                    <ul>
                `;
				if (props.networks) {
					Object.entries(props.networks).forEach(([network, ip]) => {
						html += `<li>${network}: ${ip}</li>`;
					});
				}
				html += '</ul>';
				break;

			case 'volume':
				html += `
                    <p><strong>Размер:</strong> ${props.size} GB</p>
                    <p><strong>Тип:</strong> ${props.volume_type || 'Неизвестно'}</p>
                    <p><strong>Загрузочный:</strong> ${props.bootable ? 'Да' : 'Нет'}</p>
                `;
				if (props.attached_to) {
					html += `<p><strong>Подключен к:</strong> ${props.attached_to}</p>`;
				}
				if (props.attachments && props.attachments.length > 0) {
					html += `<p><strong>Детали подключения:</strong></p><ul>`;
					props.attachments.forEach(attachment => {
						html += `<li>Сервер: ${attachment.server_name || attachment.server_id}`;
						if (attachment.device) html += ` (${attachment.device})`;
						html += `</li>`;
					});
					html += '</ul>';
				}
				break;

			case 'floating_ip':
				html += `
                    <p><strong>IP адрес:</strong> ${props.floating_ip}</p>
                    <p><strong>Сеть:</strong> ${props.floating_network_id}</p>
                `;
				if (props.fixed_ip) {
					html += `<p><strong>Привязан к IP:</strong> ${props.fixed_ip}</p>`;
				}
				if (props.attached_resource_name) {
					html += `<p><strong>Привязан к ресурсу:</strong> ${props.attached_resource_name}</p>`;
				}
				break;

			case 'vpn_service':
				html += `
                    <p><strong>Описание:</strong> ${props.description || 'Не указано'}</p>
                    <p><strong>Router ID:</strong> ${props.router_id}</p>
                `;
				if (props.subnet_id) {
					html += `<p><strong>Subnet ID:</strong> ${props.subnet_id}</p>`;
				}
				if (props.peer_id) {
					html += `<p><strong>Peer ID:</strong> ${props.peer_id}</p>`;
				}
				if (props.peer_address) {
					html += `<p><strong>Peer Address:</strong> ${props.peer_address}</p>`;
				}
				if (props.auth_mode) {
					html += `<p><strong>Auth Mode:</strong> ${props.auth_mode}</p>`;
				}
				if (props.ike_version) {
					html += `<p><strong>IKE Version:</strong> ${props.ike_version}</p>`;
				}
				if (props.mtu && props.mtu > 0) {
					html += `<p><strong>MTU:</strong> ${props.mtu}</p>`;
				}
				break;

			case 'load_balancer':
				html += `
                    <p><strong>VIP адрес:</strong> ${props.vip_address}</p>
                    <p><strong>Статус провизионирования:</strong> ${props.provisioning_status}</p>
                    <p><strong>Операционный статус:</strong> ${props.operating_status}</p>
                `;
				break;
		}

		html += '</div>';
		return html;
	}

	updateSummary() {
		if (!this.data || !this.data.summary) return;

		const summary = this.data.summary;

		document.getElementById('totalProjects').textContent = summary.total_projects || 0;
		document.getElementById('totalServers').textContent = summary.total_servers || 0;
		document.getElementById('totalVolumes').textContent = summary.total_volumes || 0;

		const networkTotal = (summary.total_floating_ips || 0) +
			(summary.total_routers || 0) +
			(summary.total_load_balancers || 0) +
			(summary.total_vpn_services || 0);
		document.getElementById('totalNetwork').textContent = networkTotal;
	}

	showLastUpdate() {
		if (this.data && this.data.generated_at) {
			const lastUpdate = new Date(this.data.generated_at);
			const lastUpdateInfo = document.getElementById('lastUpdateInfo');
			const lastUpdateText = document.getElementById('lastUpdateText');

			lastUpdateText.textContent = `Последнее обновление: ${lastUpdate.toLocaleString('ru-RU')}`;
			lastUpdateInfo.style.display = 'block';
		}
	}

	showLoading(show) {
		const spinner = document.getElementById('loadingSpinner');
		spinner.style.display = show ? 'block' : 'none';
	}

	showError(message) {
		const errorAlert = document.getElementById('errorAlert');
		const errorText = document.getElementById('errorText');

		errorText.textContent = message;
		errorAlert.style.display = 'block';
	}

	hideError() {
		document.getElementById('errorAlert').style.display = 'none';
	}

	getStatusClass(status) {
		const statusLower = status.toLowerCase();
		if (statusLower.includes('active') || statusLower.includes('available')) return 'status-active';
		if (statusLower.includes('error') || statusLower.includes('failed')) return 'status-error';
		if (statusLower.includes('building') || statusLower.includes('pending')) return 'status-building';
		if (statusLower.includes('shutoff') || statusLower.includes('down')) return 'status-shutoff';
		return 'status-active';
	}

	getTypeClass(type) {
		return `type-${type}`;
	}

	getTypeDisplayName(type) {
		const types = {
			'server': 'Виртуальная машина',
			'volume': 'Диск',
			'floating_ip': 'Floating IP',
			'router': 'Роутер',
			'load_balancer': 'Балансировщик',
			'vpn_service': 'VPN сервис',
			'cluster': 'Kubernetes кластер'
		};
		return types[type] || type;
	}

	getGroupIcon(groupBy) {
		const icons = {
			'project': 'folder',
			'type': 'layer-group',
			'status': 'circle'
		};
		return icons[groupBy] || 'list';
	}

	getResourceSubtitle(resource) {
		const props = resource.properties;

		switch (resource.type) {
			case 'server':
				// Показываем Flavor и IP адреса сетей
				let flavor_name = props.flavor_name || 'Unknown Flavor';
				let server_ip = '';

				if (props.networks && typeof props.networks === 'object') {
					const ips = Object.values(props.networks);
					if (ips.length > 0) {
						server_ip += ', ' + ips.join(', ');
					}
				}
				let subtitle = 'Flavor: ' + flavor_name + ', IPs: ' + server_ip;
				return subtitle;

			case 'volume':
				// Показываем к какой ВМ подключен
				let volume_type = props.volume_type || '❓';
				let volume_bootable = props.bootable ? '✅' : '➖';
				let volume_attached_to = props.attached_to || '❓';
				let volume_size = props.size || '❓';

				if (props.attached_to) {
					return `Type: ${volume_type}, Boot: ${volume_bootable}, Attached To: ${volume_attached_to}, Size: ${volume_size} GB`;
				}
				return `Type: ${volume_type}, Boot: ${volume_bootable}, Size: ${volume_size} GB`;

			case 'floating_ip':
				// Показываем к чему подключен
				if (props.attached_resource_name) {
					return `Подключен к: ${props.attached_resource_name}`;
				}
				return 'Не подключен';

			case 'load_balancer':
				// Показываем внутренний IP (и внешний если есть)
				let ips = [];
				if (props.vip_address) {
					ips.push(props.vip_address);
				}
				if (props.floating_ip && props.floating_ip !== props.vip_address) {
					ips.push(props.floating_ip);
				}
				return ips.length > 0 ? ips.join(', ') : 'Нет IP';

			case 'vpn_service':
				// Показываем Peer Address
				return props.peer_address || 'Нет Peer Address';

			default:
				// Для остальных типов показываем ID
				return resource.id;
		}
	}
}

// Initialize app when DOM is loaded
document.addEventListener('DOMContentLoaded', () => {
	window.app = new OpenStackReporter();
});
