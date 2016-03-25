var Market = React.createClass({
	getInitialState: function() {
		return { services: [] };
	},

	componentDidMount: function() {
		this.refreshMarket();
		if(this.props.interval !== undefined) {
			setInterval(this.refreshMarket, this.props.interval);
		}
	},
	
	refreshMarket: function() {
		this.props.ajax(this.props.apiurl + "/services", function(services) {
			this.setState({services: services});
		}.bind(this));
	},

	render: function() {
		if (this.props.hide) {
			return null
		}

		return (
			<div className="panel panel-default">
				<div className="panel-heading">
					<h3 className="panel-title">API Market</h3>
				</div>
				<div className="panel-body">
					The API store in its full glory and awesomeness...
				</div>

				<table className="table">
					<thead>
						<tr>
							<th>Name</th>
							<th>Endpoint</th>
							<th>Owner</th>
							<th>Price</th>
							<th>Cancellation time</th>
							<th></th>
						</tr>
					</thead>

					<tbody>
						{this.state.services.map(function(service, i) {
							return (
								<MarketItem account={this.props.account} key={"market-item-"+i} name={service.name} apiurl={this.props.apiurl} ajax={this.props.ajax} service={service}/>
							);
						}.bind(this))}
					</tbody>
				</table>
			</div>
		);
	}
});
window.Market = Market;

var MarketItem = React.createClass({
	getInitialState: function() {
		return { 
			working: false,
			subscribed: false,
			failure: null,
		};
	},

	subscribe: function(ev) {
		this.setState({working: true});

		var form = new FormData();
		form.append("sender", this.props.account);
		form.append("serviceId", this.props.service.serviceId);

		$.ajax({type: "POST", url: this.props.apiurl + "/subscriptions", data: form,  cache: false, processData: false, contentType: false, dataType: 'json',
			success: function(data) {
				console.log(data);
				this.setState({working: false, subscribed: true});
			}.bind(this),
			error: function(xhr, status, err) {
				this.setState({working: false, failure: xhr.responseText});
			}.bind(this),
		});
	},

	render: function() {
		return (
			<tr>
				<td>{this.props.service.name}</td>
				<td><a href={this.props.service.endpoint}>{this.props.service.endpoint}</a></td>
				<td>{this.props.service.owner}</td>
				<td>{this.props.service.price}</td>
				<td>{this.props.service.cancellationTime}</td>
				<td>
					<button disabled={this.state.working} type="submit" onClick={this.subscribe} className="btn btn-default">
						{ this.state.working ? <i className="fa fa-spinner fa-spin"></i> : null} Subscribe
					</button>
				</td>
			</tr>
		);
	},
});

